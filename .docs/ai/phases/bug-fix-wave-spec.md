# bug-fix-wave — spec (2026-06-10)

Scope: the critical + 8 high findings from the bug bash audit. Full traced
descriptions + fixes: `.harness/20260610-bug-bash-audit/report.json` (table
blocks "critical + high"). Read the finding's trace before each item — this
spec holds sequencing + acceptance, not the code.

Sequencing rule: item 1 (jsonfile helper) first — items 2–4 build on it.
Frontend items (8–9) are independent and can interleave. Tag **v0.2.5** when
all boxes in current-state.md `## Plan` are checked.

## Items

1. **`internal/jsonfile` helper + migrate all writers.**
   New package: `Patch(path string, mutate func(doc map[string]any) error)
   error` + `AtomicWrite(path string, data []byte, perm os.FileMode) error`.
   Requirements (spec-level): unique temp names via os.CreateTemp in the same
   dir (never a fixed `path+".tmp"`); decode with `dec.UseNumber()` so
   untouched numbers round-trip byte-for-byte; an existing-but-unparseable
   file is an ERROR, never an empty map (`fs.ErrNotExist` is the only
   start-fresh case). Migrate: server/report_actions.go patchReport,
   mcp/tools.go toolUpdateStatus + toolUpdateLive + atomicWrite,
   server/notifications.go saveNotifications, cmd register.go
   loadConfigMap/writeConfigMap. Mirror each site's existing nil-deletes-key
   semantics (report_actions.go:24-29). Export `manifest.ValidStatus` to kill
   the duplicated status enum in tools.go:346.
   Acceptance: zero remaining `+ ".tmp"` literals; a corrupt config.json
   makes notification-save/register error out, not clobber.

2. **respond.Record atomicity + serialization.** Route the write through
   jsonfile.AtomicWrite; add a package-level mutex around Load→write.
   Stop swallowing the respond.Load error in store.loadEntry (store.go:201)
   — surface into the scan errs instead of resetting OpenAsks.

3. **Cross-process report.json locking.** Unique temp names land via item 1;
   add the advisory flock (O_CREATE + syscall.Flock LOCK_EX on
   `<dir>/report.json.lock`) shared by patchReport and the MCP update tools
   so dashboard close/archive can't be reverted by update_live. stdlib
   syscall only.

4. **MCP list_reports scan_roots parity.** Build roots exactly the way
   server.enabledRoots does (projects.NewManager(cfg.ScanRoots, cfg.Projects,
   projects.StatePath()).Enabled()) — find and mirror that call, don't
   re-derive. Acceptance: a report under a scan_roots-discovered project
   appears in list_reports.

5. **DELETE blast-radius guard.** Before RemoveAll(entry.Dir): refuse when
   entry.Dir equals the expanded CentralDir or any enabled project's
   `.harness` root, and abort if a walk of entry.Dir finds a report.json
   other than entry.Path. On refusal: 409 + actionable message.

6. **Lenient known-block parse.** manifest.go:147 — body decode failure
   leaves Body nil + returns nil (matching the unknown-type path); renderer
   already falls back on nil Body. Keep Validate strict via its independent
   strictDecode. Test: `"pct":"50"` parses with Body nil, Validate flags it,
   store still indexes the report.

7. **Markdown link scheme allowlist.** markdown.go:52 — ReplaceAllStringFunc;
   allow http/https/mailto/relative/anchor; anything else renders as escaped
   text. Case-insensitive scheme compare.

8. **live.js draft guard.** Skip the sig-reload when document.activeElement
   is INPUT/TEXTAREA or any .hd-input is non-empty; surface a passive
   "report updated — r to refresh" affordance instead. Verify: human
   (browser; type into an ask while update_live fires).

9. **g-chord single owner.** tabs.js owns the chord; registration API
   (window.HDKeys.chord) for aggregator's dashboard-only completions; add a
   second-g case that disarms (kills the gg→a double-dispatch); document the
   keyboard precedence = ReportJS concatenation order in assets.go. Verify:
   human (browser; g t / g T / g x on dashboard, gg then a on a report).

## Report

Write `bug-fix-wave-report.md` here when done: what shipped, deviations,
which findings were re-classified (if any), the v0.2.5 tag.
