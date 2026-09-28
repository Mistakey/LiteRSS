// Vite 开发服务器把 /api 代理到开发实例（docs/specs/literss-rb0/spec.md D5）。
// 开发实例只放行与自身源严格相等的 Origin，所以只把 Vite 页面自己的源改写为目标源；
// 其他 Origin 原样转发，照旧被拒绝。
export const DEV_SERVER_ORIGIN = 'http://127.0.0.1:5173';
export const DEV_API_TARGET = 'http://127.0.0.1:1235';

export function rewriteDevOrigin(origin, target = DEV_API_TARGET) {
  return origin === DEV_SERVER_ORIGIN ? target : origin;
}

export function devApiProxy(target = DEV_API_TARGET) {
  return {
    target,
    changeOrigin: true,
    configure(proxy) {
      proxy.on('proxyReq', (proxyReq, req) => {
        const origin = req.headers.origin;
        if (origin) proxyReq.setHeader('Origin', rewriteDevOrigin(origin, target));
      });
    },
  };
}
