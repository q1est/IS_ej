import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
const source = await readFile(new URL("./server-readiness.js", import.meta.url), "utf8");
const { createServerReadiness } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

test("cold start retries health only and shares a single readiness cycle", async () => {
 const states = [], calls = [];
 let clock = 0;
 const readiness = createServerReadiness({ baseURL: "https://api.test", now: () => clock, pause: async ms => { clock += ms; }, onState: state => states.push(state), fetchImpl: async (url, options) => {
  calls.push({url, options});
  if (calls.length === 1) return new Response("Starting", {status:503});
  if (calls.length === 2) return new Response("<html>Starting</html>", {status:200});
  return Response.json({status:"ok"});
 } });
 const first = readiness.wait();
 assert.equal(first, readiness.wait());
 await first;
 assert.equal(calls.length, 3);
 assert(calls.every(call => call.url === "https://api.test/healthz" && call.options.method === "GET"));
 assert(calls.every(call => !call.options.headers.Authorization && call.options.credentials === "omit"));
 assert.equal(states.at(-1).status, "ready");
 await readiness.wait();
 assert.equal(calls.length, 3);
 clock += 30001;
 await readiness.wait();
 assert.equal(calls.length, 4);
 readiness.invalidate();
 await readiness.wait();
 assert.equal(calls.length, 5);
});

test("readiness stops at its deadline and a manual retry starts a new cycle", async () => {
 let clock = 0, healthy = false;
 const states = [];
 const readiness = createServerReadiness({baseURL:"https://api.test", budgetMs:60000, now:()=>clock, pause:async ms=>{clock+=ms;}, onState:state=>states.push(state), fetchImpl:async()=> healthy ? Response.json({status:"ok"}) : new Response("Unavailable",{status:503})});
 await assert.rejects(readiness.wait(), {code:"server_unavailable"});
 assert.equal(clock, 60000);
 assert.equal(states.at(-1).status, "unavailable");
 healthy = true;
 await readiness.wait(true);
 assert.equal(states.at(-1).status, "ready");
});

test("a hanging health request is aborted and cannot exceed the total deadline", async () => {
 let signal;
 const readiness = createServerReadiness({baseURL:"https://api.test",budgetMs:30,attemptMs:10,fetchImpl:async(url,options)=>{signal=options.signal;return new Promise(()=>{});}});
 await assert.rejects(readiness.wait(), {code:"server_unavailable"});
 assert.equal(signal.aborted, true);
});

test("missing configuration and denied health access fail without repeated requests", async () => {
 let calls = 0;
 const missing = createServerReadiness({baseURL:"",fetchImpl:async()=>{calls++;}});
 await assert.rejects(missing.wait(), {code:"server_configuration"});
 assert.equal(calls, 0);
 const denied = createServerReadiness({baseURL:"https://api.test",fetchImpl:async()=>{calls++;return new Response("Denied",{status:403});}});
 await assert.rejects(denied.wait(), {code:"server_configuration"});
 assert.equal(calls, 1);
});
