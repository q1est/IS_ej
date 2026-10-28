#!/usr/bin/env node

import { readFile } from "node:fs/promises";

const apiBase = (process.env.NETRUN_API_URL || "https://api.netrun.io/v1").replace(/\/$/, "");
const apiKey = process.env.NETRUN_API_KEY?.trim();
const projectId = process.env.NETRUN_PROJECT_ID?.trim();
const archivePath = process.argv[2];

if (!apiKey || !projectId || !archivePath) {
  throw new Error("NETRUN_API_KEY, NETRUN_PROJECT_ID and the archive path are required");
}
if (!/^\d+$/.test(projectId)) {
  throw new Error("NETRUN_PROJECT_ID must be the numeric project id from Netrun");
}

const clientHeaders = {
  "X-Netrun-Client": "github-actions-school-journal",
  "X-Lang": "ru",
};

const exchange = await fetch(`${apiBase}/api-keys/exchange`, {
  method: "POST",
  headers: { ...clientHeaders, Authorization: `Bearer ${apiKey}` },
});
if (!exchange.ok) {
  throw new Error(`Netrun key exchange failed: HTTP ${exchange.status} ${await exchange.text()}`);
}
const exchangeBody = await exchange.json();
if (typeof exchangeBody.access_token !== "string") {
  throw new Error("Netrun key exchange did not return an access token");
}

const archive = await readFile(archivePath);
const form = new FormData();
form.set("source_kind", "archive");
form.append("files", new Blob([archive], { type: "application/zip" }), "project.zip");
form.set("comment", `GitHub Actions ${process.env.GITHUB_SHA || "manual deployment"}`);

const deploy = await fetch(`${apiBase}/projects/${projectId}/code`, {
  method: "POST",
  headers: { ...clientHeaders, Authorization: `Bearer ${exchangeBody.access_token}` },
  body: form,
});
const responseText = await deploy.text();
if (!deploy.ok) {
  throw new Error(`Netrun deployment failed: HTTP ${deploy.status} ${responseText}`);
}

let result;
try {
  result = JSON.parse(responseText);
} catch {
  result = { raw: responseText };
}
console.log(JSON.stringify({ project_id: projectId, status: result.status, host_url: result.host_url || null }));
if (result.host_url) {
  console.log(`Backend URL: ${result.host_url}`);
}
