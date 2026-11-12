// The strategist's open bet in the column (docs/SHOW.md, "Bets and callbacks"): the claim, the days
// left to its deadline, progress toward a cash or net-worth target from the money clock, and the
// outcome for SETTLED_MS after it settles. panel.js calls renderBet with a `bet` event and betClock
// with each clock and day. Styles are in panel.css.
(() => {
  "use strict";
  const SETTLED_MS = 60000;
  let ev = null, clock = null, day = 0, hideTimer = null;
  const fmt = (n) => Math.round(n || 0).toLocaleString("en-US");
  const $ = (id) => document.getElementById(id);
  const OUTCOME = { won: "Bet won ✓", lost: "Bet lost ✗", abandoned: "Bet dropped", expired: "Bet expired", due: "Bet due" };

  function text(el, s) { if (el.textContent !== s) el.textContent = s; }

  // Progress needs a number to watch: only cash and net-worth bets have one on the clock.
  function progress(b) {
    if (!clock || !b.target) return null;
    if (b.kind === "cash") return { what: "cash", have: clock.cash };
    if (b.kind === "net_worth") return { what: "net worth", have: clock.net_worth };
    return null;
  }

  function draw() {
    const box = $("bet");
    const b = ev && ev.bet;
    const settled = b && b.outcome !== "staked" && b.outcome !== "due";
    const stale = settled && ev.at && Date.now() - ev.at > SETTLED_MS;
    box.classList.toggle("hidden", !b || stale);
    if (!b || stale) { if (window.panelFit) window.panelFit(); return; }
    box.className = "box" + (settled ? " settled " + (b.outcome === "won" ? "won" : b.outcome === "lost" ? "lost" : "dropped") : "");
    text($("bet-label"), settled || b.outcome === "due" ? OUTCOME[b.outcome] || "Bet " + b.outcome : "Bet · staked day " + b.staked + (b.judged ? " · model judges" : ""));
    text($("bet-claim"), "“" + b.claim.replace(/[.]+$/, "") + "”");
    const due = $("bet-due"), left = b.by - day;
    due.classList.toggle("close", !settled && left <= 1);
    text(due, settled ? "day " + (ev.day || day) : !b.by ? "" : b.outcome === "due" ? "awaiting the verdict"
      : left > 1 ? "due day " + b.by + " · " + left + " days left" : left === 1 ? "due day " + b.by + " · 1 day left" : left === 0 ? "due today" : "past due");
    const p = progress(b), track = !!p && (!settled || b.outcome === "won" || b.outcome === "lost");
    const fact = b.fact ? b.fact.charAt(0).toUpperCase() + b.fact.slice(1) : "";
    $("bet-track").classList.toggle("hidden", !track);
    if (track) {
      const pct = Math.max(0, Math.min(100, (100 * (p.have || 0)) / b.target));
      $("bet-fill").style.width = (b.outcome === "won" ? 100 : pct).toFixed(1) + "%";
      text($("bet-now"), settled ? fact : p.what + " " + fmt(p.have) + " of " + fmt(b.target));
      text($("bet-pct"), settled ? "target " + fmt(b.target) : p.have >= b.target ? "reached" : Math.floor(pct) + " %");
    }
    text($("bet-fact"), settled && !track ? fact : "");
    clearTimeout(hideTimer);
    if (settled && ev.at) hideTimer = setTimeout(draw, Math.max(0, SETTLED_MS - (Date.now() - ev.at)) + 50);
    if (window.panelFit) window.panelFit();
  }

  window.renderBet = (e) => { ev = e && e.bet ? e : null; draw(); };
  window.betClock = (c, d) => {
    if (c) clock = c;
    if (d) day = d;
    if (ev) draw();
  };
})();
