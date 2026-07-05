// Mineshaft Pi Extension — Enhanced (with per-prompt mail check)
// Deploys the same lifecycle hooks as Claude's settings-autonomous.json
// but using pi's extension API.
//
// Events mapped:
//   session_start       → ms prime --hook (capture context)
//   before_agent_start  → inject captured context + check mail every prompt
//   tool_call           → ms tap guard pr-workflow (on git push/pr create)
//   session_shutdown    → ms costs record
//
// Enhancement over upstream: mail is checked on every prompt (throttled to
// 30s) via before_agent_start, matching Claude's UserPromptSubmit behavior.
//
// Loaded via: pi -e mineshaft-hooks.js

export default (pi) => {
  const role = (process.env.MS_ROLE || "").toLowerCase();
  let primeContext = null;
  let contextInjected = false;
  let lastMailCheck = 0;

  const shouldCheckMail = () =>
    !role.includes("witness") && !role.includes("refinery") && !role.startsWith("supervisor") && !role.includes("boot");

  // SessionStart — run ms prime and capture context for injection
  pi.on("session_start", async (event, context) => {
    try {
      const result = await pi.exec("ms", ["prime", "--hook"]);
      if (result.code === 0 && result.stdout.trim()) {
        primeContext = result.stdout.trim();
        console.error("[mineshaft] ms prime captured (" + primeContext.length + " chars)");
      } else {
        console.error("[mineshaft] ms prime returned no output (code=" + result.code + ")");
      }
    } catch (e) {
      console.error("[mineshaft] ms prime failed:", e.message);
    }

  });

  // BeforeAgentStart — inject prime context + check mail every prompt
  pi.on("before_agent_start", async (event, context) => {
    let mailContext = null;

    // Check mail on every prompt (throttled to once per 30s)
    const now = Date.now();
    if (shouldCheckMail() && now - lastMailCheck >= 30000) {
      lastMailCheck = now;
      try {
        const mailResult = await pi.exec("ms", ["mail", "check", "--inject"]);
        if (mailResult.code === 0 && mailResult.stdout.trim()) {
          mailContext = mailResult.stdout.trim();
          console.error("[mineshaft] mail check: new mail found");
        }
      } catch (e) {
        console.error("[mineshaft] per-prompt mail check failed:", e.message);
      }
    }

    // Inject prime context on first prompt
    if (primeContext && !contextInjected) {
      contextInjected = true;
      console.error("[mineshaft] injecting prime context into session");
      const result = {
        message: {
          customType: "mineshaft-prime",
          content: primeContext,
          display: false,
        },
        systemPrompt: event.systemPrompt + "\n\n" + primeContext,
      };
      if (mailContext) {
        result.systemPrompt += "\n\n" + mailContext;
        result.message.content += "\n\n" + mailContext;
      }
      return result;
    }

    // After first prompt, inject mail if present
    if (mailContext) {
      return {
        message: {
          customType: "mineshaft-mail",
          content: mailContext,
          display: false,
        },
        systemPrompt: event.systemPrompt + "\n\n" + mailContext,
      };
    }
  });

  // PreToolUse equivalent — guard dangerous git operations
  pi.on("tool_call", async (event, context) => {
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

  // Stop equivalent — record API costs
  pi.on("session_shutdown", async (event, context) => {
    try {
      await pi.exec("ms", ["costs", "record"]);
    } catch (e) {
      console.error("[mineshaft] ms costs record failed:", e.message);
    }
  });
};
