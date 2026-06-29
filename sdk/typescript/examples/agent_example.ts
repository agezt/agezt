/**
 * Example: AI Agent Subprocess Using the Agezt Agent SDK
 *
 * This example shows how an AI agent subprocess can use the Agent SDK
 * to communicate with the AGEZT kernel via scoped capability tokens.
 *
 * Usage:
 *   # Token is passed by the parent agent via environment
 *   export AGEZT_AGENT_TOKEN="eyJ..."
 *   npx ts-node agent_example.ts
 *   # or: npm run build && node dist/examples/agent_example.js
 */

import { AgentClient } from "../src/index.js";

async function main() {
  // Get token from environment (set by parent agent)
  const token = process.env.AGEZT_AGENT_TOKEN;
  if (!token) {
    console.error("ERROR: AGEZT_AGENT_TOKEN environment variable not set");
    console.error("The parent agent should set this before spawning the subprocess.");
    process.exit(1);
  }

  // Create the agent client
  const client = new AgentClient({ token });

  console.log("=== Agezt Agent SDK Demo ===\n");

  // --- Memory Operations ---
  console.log("1. Remembering a fact...");
  const record = await client.memory.write({
    type: "fact",
    subject: "Agezt Agent SDK",
    content: "The Agent SDK allows AI agents to securely communicate with AGEZT."
  });
  console.log(`   Created: ${record.id}`);

  console.log("\n2. Searching memories...");
  const results = await client.memory.search("Agezt Agent SDK");
  for (const r of results) {
    console.log(`   - [${r.score.toFixed(2)}] ${r.subject}: ${r.content.slice(0, 50)}...`);
  }

  // --- Eventbus Operations ---
  console.log("\n3. Publishing an event...");
  await client.eventbus.publish("agent.demo", {
    message: "Hello from agent subprocess!",
    timestamp: new Date().toISOString()
  });
  console.log("   Event published!");

  // --- Logging ---
  console.log("\n4. Writing logs...");
  await client.log.write("Agent subprocess started");
  await client.log.write("Processing complete", { level: "info", meta: { items: 42 } });
  console.log("   Logs written!");

  // --- Agent Operations ---
  console.log("\n5. Listing agents...");
  const agents = await client.agent.list();
  console.log(`   Found ${agents.length} agents:`);
  for (const a of agents.slice(0, 5)) {
    console.log(`   - ${a.name} (${a.model})`);
  }

  console.log("\n=== Demo Complete ===");
  console.log("\nFor AI agents writing code, use:");
  console.log("  import { AgentClient } from '@agezt/sdk';");
  console.log("  const client = new AgentClient({ token: process.env.AGEZT_AGENT_TOKEN! });");
  console.log("  await client.memory.write({ type: 'fact', subject: '...', content: '...' });");
  console.log("  const results = await client.memory.search('query');");
}

main().catch(console.error);
