import { EventEmitter } from 'node:events';
import { describe, expect, it } from 'vitest';
import { DEV_API_TARGET, DEV_SERVER_ORIGIN, devApiProxy, rewriteDevOrigin } from './devProxy.js';

describe('rewriteDevOrigin', () => {
  it('只把 Vite 开发服务器的源改写为目标源', () => {
    expect(rewriteDevOrigin(DEV_SERVER_ORIGIN)).toBe(DEV_API_TARGET);
  });

  it.each(['https://attacker.example', 'http://127.0.0.1:3000', 'http://localhost:5173', 'null'])(
    '其他 Origin 原样转发：%s',
    (origin) => {
      expect(rewriteDevOrigin(origin)).toBe(origin);
    }
  );
});

describe('devApiProxy', () => {
  function forward(origin) {
    const proxy = new EventEmitter();
    devApiProxy().configure(proxy);
    const headers = {};
    const proxyReq = { setHeader: (k, v) => (headers[k] = v) };
    proxy.emit('proxyReq', proxyReq, { headers: origin ? { origin } : {} });
    return headers;
  }

  it('改写 5173 的 Origin', () => {
    expect(forward(DEV_SERVER_ORIGIN)).toEqual({ Origin: DEV_API_TARGET });
  });

  it('不带 Origin 的请求不添加 Origin', () => {
    expect(forward(undefined)).toEqual({});
  });

  it('异源 Origin 不改', () => {
    expect(forward('https://attacker.example')).toEqual({ Origin: 'https://attacker.example' });
  });
});
