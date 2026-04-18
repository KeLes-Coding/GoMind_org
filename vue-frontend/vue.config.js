const localConfig = (() => {
  try {
    return require('./vue.config.local')
  } catch (error) {
    if (error.code !== 'MODULE_NOT_FOUND') {
      throw error
    }
    return {}
  }
})()

const backendProxyTarget =
  process.env.BACKEND_PROXY_TARGET ||
  localConfig.backendProxyTarget ||
  'http://localhost:9090'

module.exports = {
  parallel: false,
  devServer: {
    port: 8080,
    proxy: {
      '/api': {
        target: backendProxyTarget,
        changeOrigin: true,
        pathRewrite: {
          '^/api': '/api/v1'
        }
      }
    }
  }
}
