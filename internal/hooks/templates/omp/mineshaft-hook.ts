// Mineshaft oh-my-pi (omp) hook — lifecycle integration for Mineshaft agents.
// Mirrors the same events as Claude's settings-autonomous.json and pi-mono's mineshaft-hooks.js.
// Inspired by ProbabilityEngineer/pi-mono mineshaft integration:
// https://github.com/ProbabilityEngineer/pi-mono
//
// Events mapped:
//   session_start       → ms prime --hook (capture context)
//   before_agent_start  → inject captured context + check mail every prompt
//   session.compacting  → inject compaction recovery instructions
//   tool_call           → ms tap guard pr-workflow (on git push/pr create)
//   session_shutdown    → ms costs record
//
// Loaded via: omp --hook mineshaft-hook.ts

export default function (pi) {
  const role = (process.env.MS_ROLE || "").toLowerCase();
  const shouldCheckMail = () =>
    !role.includes("witness") && !role.includes("refinery") && !role.startsWith("supervisor") && !role.includes("boot");
  let primeContext = null;
  let contextInjected = false;
  let lastMailCheck = 0;

  // SessionStart — run ms prime and capture context for injection.
  pi.on("session_start", async (event, ctx) => {
    try {
      const result = await pi.exec("ms", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout?.trim()) {
        primeContext = result.stdout.trim();
        console.error("[mineshaft] ms prime captured (" + primeContext.length + " chars)");
      } else {
        console.error("[mineshaft] ms prime returned no output (code=" + result.code + ")");
      }
    } catch (e) {
      console.error("[mineshaft] ms prime failed:", e.message);
    }

  });

  // BeforeAgentStart — inject prime context + check mail every prompt.
  pi.on("before_agent_start", async (event, ctx) => {
    let mailContext = null;

    // Check mail on every prompt (throttled to once per 30s) for non-patrol roles.
    if (shouldCheckMail()) {
      const now = Date.now();
      if (now - lastMailCheck >= 30000) {
        lastMailCheck = now;
        try {
          const mailResult = await pi.exec("ms", ["mail", "check", "--inject"]);
          if (mailResult.code === 0 && mailResult.stdout?.trim()) {
            mailContext = mailResult.stdout.trim();
            console.error("[mineshaft] mail check: new mail found");
          }
        } catch (e) {
          console.error("[mineshaft] per-prompt mail check failed:", e.message);
        }
      }
    }

    // Inject prime context on first prompt.
    if (primeContext && !contextInjected) {
      contextInjected = true;
      console.error("[mineshaft] injecting prime context into session");
      const result = {
        message: {
          customType: "mineshaft-prime",
          content: primeContext,
          display: false,
        },
        systemPrompt: (event.systemPrompt || "") + "\n\n" + primeContext,
      };
      if (mailContext) {
        result.systemPrompt += "\n\n" + mailContext;
        result.message.content += "\n\n" + mailContext;
      }
      return result;
    }

    // After first prompt, inject mail if present.
    if (mailContext) {
      return {
        message: {
          customType: "mineshaft-mail",
          content: mailContext,
          display: false,
        },
        systemPrompt: (event.systemPrompt || "") + "\n\n" + mailContext,
      };
    }
  });

  // Compaction — reload prime context after compaction so the agent recovers.
  pi.on("session_compact", async (event, ctx) => {
    contextInjected = false;
    primeContext = null;
    try {
      const result = await pi.exec("ms", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout?.trim()) {
        primeContext = result.stdout.trim();
        console.error("[mineshaft] prime context refreshed after compaction");
      }
    } catch (e) {
      console.error("[mineshaft] ms prime refresh failed:", e.message);
    }
  });

  // PreToolUse — guard dangerous git operations via ms tap.
  pi.on("tool_call", async (event, ctx) => {
    if (event.toolName === "bash" && event.input?.command) {
      const cmd = event.input.command;
      if (
        cmd.includes("git push") ||
        cmd.includes("gh pr create") ||
        cmd.includes("git checkout -b")
      ) {
        try {
          const result = await pi.exec("ms", ["tap", "guard", "pr-workflow"]);
          if (result.code !== 0) {
            return { block: true, reason: result.stderr || "ms tap guard rejected this operation" };
          }
        } catch (e) {
          console.error("[mineshaft] ms tap guard failed:", e.message);
        }
      }
    }
  });

  // Shutdown — record API costs.
  pi.on("session_shutdown", async (event, ctx) => {
    try {
      await pi.exec("ms", ["costs", "record"]);
    } catch (e) {
      console.error("[mineshaft] ms costs record failed:", e.message);
    }
  });
}
