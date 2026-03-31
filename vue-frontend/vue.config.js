module.exports = {
  devServer: {
    port: 8080,
    proxy: {
      '/api': {
        target: 'http://localhost:19090',
        changeOrigin: true,
        pathRewrite: {
          '^/api': '/api/v1'
        }
      }
    }
  }
}