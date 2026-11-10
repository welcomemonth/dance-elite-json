/**
 * @name 代理的配置
 * @see 在生产环境 代理是无法生效的，所以这里没有生产环境的配置
 * -------------------------------
 * The agent cannot take effect in the production environment
 * so there is no configuration of the production environment
 * For details, please see
 * https://pro.ant.design/docs/deploy
 *
 * @doc https://umijs.org/docs/guides/proxy
 *
 * 【接入真实后端】前端请求一律走 /api/v1/admin/** 与 /h5/** 相对路径。
 * 开发环境：MOCK=none 启动（package.json 的 start 脚本已默认）+ 把下方
 * dev 代理的 target 指向 Go 服务（默认 http://localhost:8080）即可，
 * mock（mock/ 目录）与真实后端接口一一对应，无需改任何业务代码。
 */
export default {
  dev: {
    // localhost:8000/api/** -> Go 后端 http://localhost:8080/api/**
    '/api/': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
    // 音频审核 H5 接口
    '/h5/': {
      target: 'http://localhost:8080',
      changeOrigin: true,
    },
  },
  test: {
    // localhost:8000/api/** -> https://pro-api.ant-design-demo.workers.dev/api/**
    '/api/': {
      target: 'https://pro-api.ant-design-demo.workers.dev',
      changeOrigin: true,
    },
  },
  pre: {
    // localhost:8000/api/** -> your pre url
    '/api/': {
      target: 'your pre url',
      changeOrigin: true,
    },
  },
};
