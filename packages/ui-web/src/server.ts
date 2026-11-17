import indexHTML from "./index.html";

const port = Number(process.env.PORT) || 3000;
// Bun inlines this public variable in browser bundles, including outside devhost.
process.env.AGENT_PUBLIC_API_URL ??= "";

const server = Bun.serve({
  port,
  hostname: process.env.DEVHOST_BIND_HOST,
  routes: {
    "/": indexHTML,
  },
  development: process.env.NODE_ENV !== "production",
  fetch(_req) {
    return new Response("Not Found", { status: 404 });
  },
});

const isAgent = process.env.AGENT === "1" || process.env.AGENT === "true" || process.env.AGENT === "yes";
if (isAgent) {
  console.log(`[ui] INFO: running on port ${server.port}`);
} else {
  console.log(`[INFO] Agent Status UI server running at http://localhost:${server.port}`);
}
