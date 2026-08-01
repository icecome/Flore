const CACHE_NAME = 'flore-v1';
const STATIC_ASSETS = [
  '/',
  '/index.html',
  '/manifest.json',
];

// 安装时缓存核心资源
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(STATIC_ASSETS).catch(() => {
        // 部分资源可能不存在，忽略错误
      });
    })
  );
  self.skipWaiting();
});

// 激活时清理旧缓存
self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys
          .filter((key) => key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      );
    })
  );
  self.clients.claim();
});

// 拦截请求：静态资源优先使用缓存，API 请求使用网络优先
self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // 不缓存非 GET 请求
  if (request.method !== 'GET') {
    return;
  }

  // API 请求：网络优先，失败时返回缓存
  // 仅按 pathname 判断，不依赖硬编码端口（端口可能因环境变化）
  if (url.pathname.startsWith('/api/')) {
    event.respondWith(
      fetch(request)
        .then((response) => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then((cache) => cache.put(request, clone));
          return response;
        })
        .catch(() => caches.match(request))
    );
    return;
  }

  // 静态资源：缓存优先
  event.respondWith(
    caches.match(request).then((cached) => {
      if (cached) {
        // 后台更新缓存
        fetch(request)
          .then((response) => {
            caches.open(CACHE_NAME).then((cache) => cache.put(request, response));
          })
          .catch(() => {});
        return cached;
      }
      return fetch(request).then((response) => {
        const clone = response.clone();
        caches.open(CACHE_NAME).then((cache) => cache.put(request, clone));
        return response;
      });
    })
  );
});
