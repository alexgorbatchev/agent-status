import indexHTML from "./index.html";

const port = Number(process.env.PORT) || 3000;

const server = Bun.serve({
  port,
  routes: {
    "/": indexHTML,
  },
  development: process.env.NODE_ENV !== "production",
  fetch(_req) {
    return new Response("Not Found", { status: 404 });
  },
});

console.log(`🚀 Agent Status UI server running at http://localhost:${server.port}`);
