package mcpgateway

import (
	"GopherAI/common/applog"
	"GopherAI/config"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// TransportStreamableHTTP 表示基于 Streamable HTTP 的 MCP 连接。
	TransportStreamableHTTP = "streamable_http"
	// TransportHTTP 作为旧称兼容，内部统一映射为 Streamable HTTP。
	TransportHTTP = "http"
	// TransportStdio 表示通过本地进程 stdio 建立 MCP 连接。
	TransportStdio = "stdio"
)

var (
	globalManager *Manager
	managerOnce   sync.Once
)

// ToolDefinition 表示聚合后的工具元数据。
// QualifiedName 用于解决跨 Server 同名工具冲突，AliasName 只在唯一时暴露。
type ToolDefinition struct {
	ServerName    string
	ToolName      string
	QualifiedName string
	AliasName     string
	Description   string
	Origin        string
	InputSchema   string
}

// Manager 负责统一管理多个 MCP Server 的 client、工具发现和调用路由。
type Manager struct {
	mu              sync.RWMutex
	servers         map[string]*serverEntry
	qualifiedRoutes map[string]*toolRoute
	aliasRoutes     map[string]*toolRoute
	tools           []ToolDefinition
	defaultServer   string
}

type serverEntry struct {
	config config.MCPServerConfig
	client *client.Client
	tools  []mcp.Tool
}

type toolRoute struct {
	serverName string
	toolName   string
}

// GetGlobalManager 返回全局 MCP 聚合管理器。
// 第一阶段按静态配置初始化；后续如需热更新可在此基础上继续演进。
func GetGlobalManager() *Manager {
	managerOnce.Do(func() {
		globalManager = NewManager(config.GetConfig().MCPConfig)
	})
	return globalManager
}

// NewManager 根据配置创建一个 MCP 聚合管理器。
func NewManager(cfg config.MCPConfig) *Manager {
	return &Manager{
		servers:         buildServerEntries(cfg.EffectiveServers()),
		qualifiedRoutes: make(map[string]*toolRoute),
		aliasRoutes:     make(map[string]*toolRoute),
		defaultServer:   strings.TrimSpace(cfg.DefaultServer),
	}
}

func buildServerEntries(servers []config.MCPServerConfig) map[string]*serverEntry {
	entries := make(map[string]*serverEntry, len(servers))
	for _, server := range servers {
		entries[server.Name] = &serverEntry{config: server}
	}
	return entries
}

// IsEnabled 判断当前是否至少配置了一个可用 MCP Server。
func (m *Manager) IsEnabled() bool {
	if m == nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.servers) > 0
}

// ListTools 返回当前聚合后的工具清单。
// 首次调用会触发一次初始化和 tools 发现。
func (m *Manager) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	if err := m.EnsureInitialized(ctx); err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	tools := make([]ToolDefinition, len(m.tools))
	copy(tools, m.tools)
	return tools, nil
}

// EnsureInitialized 负责初始化所有 Server 并构建工具路由。
func (m *Manager) EnsureInitialized(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("mcp manager is nil")
	}

	m.mu.RLock()
	initialized := len(m.tools) > 0 || len(m.servers) == 0
	m.mu.RUnlock()
	if initialized {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tools) > 0 || len(m.servers) == 0 {
		return nil
	}

	type discoveredTool struct {
		def   ToolDefinition
		route *toolRoute
	}
	var discovered []discoveredTool
	for serverName, entry := range m.servers {
		tools, err := m.initializeServer(ctx, entry)
		if err != nil {
			applog.Categoryf(applog.CategoryMCP, "MCP server init failed server=%s transport=%s err=%v", serverName, entry.config.Transport, err)
			continue
		}
		for _, tool := range tools {
			if !isToolAllowed(entry.config.ToolAllowlist, entry.config.ToolBlocklist, tool.Name) {
				continue
			}
			def := ToolDefinition{
				ServerName:    serverName,
				ToolName:      tool.Name,
				QualifiedName: qualifyToolName(serverName, tool.Name),
				Description:   strings.TrimSpace(tool.Description),
				Origin:        entry.config.Origin,
				InputSchema:   marshalToolSchema(tool),
			}
			discovered = append(discovered, discoveredTool{
				def: def,
				route: &toolRoute{
					serverName: serverName,
					toolName:   tool.Name,
				},
			})
		}
	}

	if len(discovered) == 0 {
		return fmt.Errorf("no MCP tools available")
	}

	m.tools = make([]ToolDefinition, 0, len(discovered))
	nameCount := make(map[string]int)
	for _, item := range discovered {
		nameCount[item.def.ToolName]++
		m.qualifiedRoutes[item.def.QualifiedName] = item.route
	}
	for _, item := range discovered {
		if nameCount[item.def.ToolName] == 1 {
			item.def.AliasName = item.def.ToolName
			m.aliasRoutes[item.def.ToolName] = item.route
		}
		m.tools = append(m.tools, item.def)
	}

	sort.Slice(m.tools, func(i, j int) bool {
		if m.tools[i].ServerName == m.tools[j].ServerName {
			return m.tools[i].ToolName < m.tools[j].ToolName
		}
		return m.tools[i].ServerName < m.tools[j].ServerName
	})
	return nil
}

func (m *Manager) initializeServer(ctx context.Context, entry *serverEntry) ([]mcp.Tool, error) {
	if entry.client == nil {
		clientInstance, err := newMCPClient(entry.config)
		if err != nil {
			return nil, err
		}
		entry.client = clientInstance
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(entry.config.TimeoutSeconds)*time.Second)
	defer cancel()

	if !entry.client.IsInitialized() {
		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "GoMind MCP Gateway",
			Version: "1.0.0",
		}
		initRequest.Params.Capabilities = mcp.ClientCapabilities{}

		if _, err := entry.client.Initialize(timeoutCtx, initRequest); err != nil {
			return nil, fmt.Errorf("initialize failed: %w", err)
		}
	}

	result, err := entry.client.ListTools(timeoutCtx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("list tools failed: %w", err)
	}
	entry.tools = result.Tools
	return result.Tools, nil
}

func newMCPClient(cfg config.MCPServerConfig) (*client.Client, error) {
	switch normalizeTransport(cfg.Transport) {
	case TransportStreamableHTTP:
		return client.NewStreamableHttpClient(
			cfg.BaseURL,
			transport.WithHTTPTimeout(time.Duration(cfg.TimeoutSeconds)*time.Second),
			transport.WithHTTPHeaders(cfg.Headers),
		)
	case TransportStdio:
		return client.NewStdioMCPClient(cfg.Command, nil, cfg.Args...)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", cfg.Transport)
	}
}

// CallTool 调用聚合后的工具。
// 优先按完全限定名查找，其次在工具名唯一时允许直接按短名调用。
func (m *Manager) CallTool(ctx context.Context, toolName string, args map[string]any) (string, error) {
	if err := m.EnsureInitialized(ctx); err != nil {
		return "", err
	}

	m.mu.RLock()
	route := m.resolveRouteLocked(toolName)
	if route == nil {
		m.mu.RUnlock()
		return "", fmt.Errorf("tool not found: %s", toolName)
	}
	entry := m.servers[route.serverName]
	m.mu.RUnlock()
	if entry == nil || entry.client == nil {
		return "", fmt.Errorf("server unavailable for tool: %s", toolName)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(entry.config.TimeoutSeconds)*time.Second)
	defer cancel()

	callToolRequest := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      route.toolName,
			Arguments: args,
		},
	}
	startAt := time.Now()
	result, err := entry.client.CallTool(timeoutCtx, callToolRequest)
	latency := time.Since(startAt).Milliseconds()
	if err != nil {
		applog.Categoryf(applog.CategoryMCP, "MCP tool call failed server=%s tool=%s latency_ms=%d err=%v", route.serverName, route.toolName, latency, err)
		return "", fmt.Errorf("mcp tool call failed: %w", err)
	}

	text := extractToolResultText(result)
	text = truncateToolResult(text, entry.config.MaxResultChars)
	applog.Categoryf(applog.CategoryMCP, "MCP tool call success server=%s tool=%s latency_ms=%d result_size=%d", route.serverName, route.toolName, latency, len(text))
	return text, nil
}

func (m *Manager) resolveRouteLocked(toolName string) *toolRoute {
	if route, ok := m.qualifiedRoutes[toolName]; ok {
		return route
	}
	if route, ok := m.aliasRoutes[toolName]; ok {
		return route
	}
	if m.defaultServer != "" && !strings.Contains(toolName, ".") {
		if route, ok := m.qualifiedRoutes[qualifyToolName(m.defaultServer, toolName)]; ok {
			return route
		}
	}
	return nil
}

// isToolAllowed 统一处理白名单和黑名单。
// 白名单优先表达“只开放这些工具”，黑名单表达“这些工具即使被发现也不暴露”。
func isToolAllowed(allowlist []string, blocklist []string, toolName string) bool {
	for _, blocked := range blocklist {
		if strings.TrimSpace(blocked) == toolName {
			return false
		}
	}
	if len(allowlist) == 0 {
		return true
	}
	for _, allowed := range allowlist {
		if strings.TrimSpace(allowed) == toolName {
			return true
		}
	}
	return false
}

func qualifyToolName(serverName, toolName string) string {
	return fmt.Sprintf("%s.%s", serverName, toolName)
}

func normalizeTransport(transportName string) string {
	switch strings.ToLower(strings.TrimSpace(transportName)) {
	case "", TransportHTTP, TransportStreamableHTTP:
		return TransportStreamableHTTP
	case TransportStdio:
		return TransportStdio
	default:
		return strings.ToLower(strings.TrimSpace(transportName))
	}
}

func marshalToolSchema(tool mcp.Tool) string {
	rawSchema := tool.RawInputSchema
	if len(rawSchema) == 0 {
		bytes, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return ""
		}
		return string(bytes)
	}
	return string(rawSchema)
}

func extractToolResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	var builder strings.Builder
	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			builder.WriteString(textContent.Text)
			if !strings.HasSuffix(textContent.Text, "\n") {
				builder.WriteString("\n")
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

// truncateToolResult 对第三方工具结果做统一长度限制。
// 第一阶段先按字符数截断，优先保证聊天上下文不会被超长工具结果直接冲爆。
func truncateToolResult(text string, maxChars int) string {
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	ellipsis := "\n...[结果已截断]"
	if maxChars <= len(ellipsis) {
		return text[:maxChars]
	}
	return text[:maxChars-len(ellipsis)] + ellipsis
}
