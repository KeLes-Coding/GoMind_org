package utils

import (
	"GopherAI/model"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func GetRandomNumbers(num int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	code := ""
	for i := 0; i < num; i++ {
		digit := r.Intn(10)
		code += strconv.Itoa(digit)
	}
	return code
}

// MD5 保留给历史密码兼容逻辑使用，新注册不再写入 MD5。
func MD5(str string) string {
	m := md5.New()
	m.Write([]byte(str))
	return hex.EncodeToString(m.Sum(nil))
}

// HashPassword 使用 bcrypt 生成密码哈希，用于新注册用户。
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// VerifyPassword 校验 bcrypt 哈希是否与用户输入匹配。
func VerifyPassword(hashedPassword, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil
}

func GenerateUUID() string {
	return uuid.New().String()
}

// ConvertToModelMessage 把 schema.Message 转成数据库可存储的消息结构。
func ConvertToModelMessage(sessionID string, userName string, msg *schema.Message) *model.Message {
	if msg == nil {
		return &model.Message{
			SessionID: sessionID,
			UserName:  userName,
		}
	}

	return &model.Message{
		SessionID:        sessionID,
		UserName:         userName,
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ResponseMeta:     MustMarshalJSONString(msg.ResponseMeta),
		Extra:            MustMarshalJSONString(msg.Extra),
	}
}

// ConvertToSchemaMessages 把数据库消息转换成 schema.Message，供 AI 模块继续使用。
func ConvertToSchemaMessages(msgs []*model.Message) []*schema.Message {
	schemaMsgs := make([]*schema.Message, 0, len(msgs))
	for _, m := range msgs {
		role := schema.Assistant
		if m.IsUser {
			role = schema.User
		}
		schemaMsgs = append(schemaMsgs, &schema.Message{
			Role:             role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
			ResponseMeta:     ParseJSONStringToResponseMeta(m.ResponseMeta),
			Extra:            ParseJSONStringToMap(m.Extra),
		})
	}
	return schemaMsgs
}

func MustMarshalJSONString(v any) string {
	if v == nil {
		return ""
	}

	payload, err := json.Marshal(v)
	if err != nil || string(payload) == "null" || string(payload) == "{}" {
		return ""
	}
	return string(payload)
}

func ParseJSONStringToMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) == 0 {
		return nil
	}
	return out
}

func ParseJSONStringToResponseMeta(raw string) *schema.ResponseMeta {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var out schema.ResponseMeta
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	if out.FinishReason == "" && out.Usage == nil && out.LogProbs == nil {
		return nil
	}
	return &out
}

// RemoveAllFilesInDir 删除目录中的所有文件，但保留子目录。
func RemoveAllFilesInDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(dir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				return err
			}
		}
	}
	return nil
}

const MaxFileSize = 10 * 1024 * 1024 // 10MB

// ValidateFile 校验上传文件是否为允许的文本文件，目前只接受 .md 和 .txt。
func ValidateFile(file *multipart.FileHeader) error {
	// 校验文件大小
	if file.Size > MaxFileSize {
		return fmt.Errorf("文件过大，最大允许 10MB，当前文件: %.2fMB", float64(file.Size)/(1024*1024))
	}

	// 校验文件类型
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".md" && ext != ".txt" {
		return fmt.Errorf("文件类型不正确，只允许 .md 或 .txt 文件，当前扩展名: %s", ext)
	}

	return nil
}

// ValidateFileMeta 为“直传初始化”场景提供元信息校验。
// 这个路径下后端拿不到 multipart 文件流，因此需要把原先基于 FileHeader 的校验拆成可复用版本。
func ValidateFileMeta(fileName string, fileSize int64) error {
	if fileSize <= 0 {
		return fmt.Errorf("文件大小必须大于 0")
	}
	if fileSize > MaxFileSize {
		return fmt.Errorf("文件过大，最大允许 10MB，当前文件 %.2fMB", float64(fileSize)/(1024*1024))
	}

	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".md" && ext != ".txt" {
		return fmt.Errorf("文件类型不正确，只允许 .md 或 .txt 文件，当前扩展名: %s", ext)
	}
	return nil
}
