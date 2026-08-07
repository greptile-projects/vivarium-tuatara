import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  timeout: 120_000,
  fullyParallel: false,
  workers: 1,
  use: {
    baseURL: "http://localhost:3000",
    headless: true,
    launchOptions: { executablePath: process.env.CHROMIUM_PATH ?? "/usr/local/bin/chromium" },
  },
  webServer: [
    {
      command: "bash -lc 'root=$(mktemp -d); GIT_STORAGE_ROOT=$root/git USER_STORAGE_ROOT=$root/users AUTH_STORAGE_ROOT=$root/auth REPOSITORY_STORAGE_ROOT=$root/repositories PROPOSAL_STORAGE_ROOT=$root/proposals PULL_REQUEST_STORAGE_ROOT=$root/pulls go run .'",
      cwd: "../api",
      url: "http://127.0.0.1:8080/health",
      timeout: 120_000,
    },
    {
      command: "bun run dev",
      url: "http://127.0.0.1:3000",
      timeout: 120_000,
    },
  ],
});
