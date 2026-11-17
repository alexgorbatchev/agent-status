import tailwindPlugin from "bun-plugin-tailwind";

process.env.AGENT_PUBLIC_API_URL ??= "";

const result = await Bun.build({
  entrypoints: ["src/main.tsx"],
  outdir: "dist",
  minify: true,
  env: "AGENT_PUBLIC_*",
  plugins: [tailwindPlugin],
});

if (!result.success) {
  for (const log of result.logs) {
    console.error(log);
  }
  process.exit(1);
}
