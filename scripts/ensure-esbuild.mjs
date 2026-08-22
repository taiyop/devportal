import { spawnSync } from "node:child_process";
import { existsSync, readdirSync } from "node:fs";
import { join } from "node:path";

// bun/npm can leave the macOS esbuild binary with a broken ad-hoc signature.
// macOS then SIGKILLs it ("Code Signature Invalid"), and Vite fails with
// "The service was stopped" while loading vite.config.ts.
if (process.platform !== "darwin") process.exit(0);

const root = join(process.cwd(), "node_modules", "@esbuild");
if (!existsSync(root)) process.exit(0);

for (const name of readdirSync(root)) {
  const bin = join(root, name, "bin", "esbuild");
  if (!existsSync(bin)) continue;

  const probe = spawnSync(bin, ["--version"], { encoding: "utf8", timeout: 8000 });
  if (probe.status === 0) continue;

  const signed = spawnSync("codesign", ["--force", "--sign", "-", bin], {
    encoding: "utf8",
  });
  if (signed.status !== 0) {
    console.error(
      `esbuild at ${bin} could not be launched (exit ${probe.status ?? probe.signal}) and codesign failed:\n${signed.stderr || signed.stdout}`,
    );
    process.exit(1);
  }

  const retry = spawnSync(bin, ["--version"], { encoding: "utf8", timeout: 8000 });
  if (retry.status !== 0) {
    console.error(
      `esbuild at ${bin} still cannot launch after re-signing (exit ${retry.status ?? retry.signal}).`,
    );
    process.exit(1);
  }
  console.log(`re-signed ${bin} (${retry.stdout.trim()})`);
}
