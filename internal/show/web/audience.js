// The Twitch audience in the column (docs/SHOW.md, "Twitch"): the channel-points prediction and the
// chat vote, each with its question, split and result written out, so a VOD with no chat beside it
// still reads. panel.js calls renderAudience with an `audience` event's payload. Styles are in
// panel.css; while the fork box shows a fork's vote, this box keeps the prediction only.
(() => {
  "use strict";

  let current = null;
  const fmt = (n) => Math.round(n || 0).toLocaleString("en-US");
  const pct = (a, b) => (b ? Math.round((100 * a) / b) : 0);

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  function left(ms) {
    const s = Math.max(0, Math.round((ms - Date.now()) / 1000));
    return Math.floor(s / 60) + ":" + String(s % 60).padStart(2, "0");
  }

  function head(kind, status) {
    const h = el("div", "ah");
    h.appendChild(el("span", "ak", kind));
    h.appendChild(el("span", "as", status));
    return h;
  }

  function prediction(p) {
    const box = el("div", "ab pred");
    const st = { active: p.locks_at ? "open · locks in " + left(p.locks_at) : "open", locked: "locked", resolved: "resolved", canceled: "refunded", failed: "not opened" }[p.status] || p.status;
    box.appendChild(head(p.about === "verdict" ? "Twitch prediction · the format" : "Twitch prediction · the bet", st));
    box.appendChild(el("div", "aq", p.title));
    const os = p.outcomes || [];
    const users = os.reduce((s, o) => s + (o.users || 0), 0);
    if (os.length === 2) {
      const bar = el("div", "split"), labels = el("div", "sl");
      os.forEach((o, i) => {
        const seg = el("div", o.won ? "won" : i === 0 ? "y" : "n");
        seg.style.width = (users ? (100 * o.users) / users : 50) + "%";
        bar.appendChild(seg);
        labels.appendChild(el("span", o.won ? "won" : "", (o.won ? "✓ " : "") + o.title + (users ? " " + pct(o.users, users) + "% · " + fmt(o.points) + " pts" : "")));
      });
      box.appendChild(bar);
      box.appendChild(labels);
    }
    if (p.result) box.appendChild(el("div", "ar good", "Result: " + p.result + (users ? " · " + fmt(users) + " predicted" : "")));
    else if (p.status === "canceled" || p.status === "failed") box.appendChild(el("div", "ar", p.note || ""));
    else if (p.claim && p.claim.replace(/[.?!]+$/, "") !== p.title.replace(/[.?!]+$/, "")) box.appendChild(el("div", "ar claim", "“" + p.claim + "”" + (p.by ? " · due day " + p.by : "")));
    if (p.skipped) box.appendChild(el("div", "ar skip", "Also staked, no prediction (" + p.skipped.why + "): “" + p.skipped.claim + "”"));
    return box;
  }

  function poll(q) {
    const box = el("div", "ab poll");
    box.appendChild(head("Chat vote · " + q.for, fmt(q.total) + " votes" + (q.status === "open" && q.closes_at ? " · " + left(q.closes_at) : " · closed")));
    box.appendChild(el("div", "aq", q.question));
    const opts = el("div", (q.options || []).length > 3 ? "opts many" : "opts");
    const top = Math.max(0, ...(q.options || []).map((o) => o.votes));
    for (const o of q.options || []) {
      const share = pct(o.votes, q.total);
      const c = el("div", "opt" + (q.total && o.votes === top ? " top" : ""), "!vote " + o.word + (q.total ? " " + share + "%" : ""));
      c.style.background = "linear-gradient(90deg, rgba(145,71,255,.35) " + share + "%, transparent " + share + "%)";
      c.title = o.label;
      opts.appendChild(c);
    }
    box.appendChild(opts);
    if (q.answer) box.appendChild(el("div", "ar ans", "Strategist: “" + q.answer + "”"));
    else if (q.fact) box.appendChild(el("div", "ar", "Sent to the strategist: " + q.fact.replace(/^Twitch chat /, "chat ").replace(/ \(.*\)\.$/, ".")));
    else if (q.note) box.appendChild(el("div", "ar", q.note));
    return box;
  }

  // The vote before, in one line under the open one: what the strategist made of it.
  function lastLine(q) {
    const said = (q.fact || q.note || "").replace(/^Twitch chat /, "chat ").replace(/ \(.*\)\.$/, "");
    return el("div", q.answer ? "ar ans last" : "ar last", "Last vote (" + q.for + "): " + (q.answer ? "strategist: “" + q.answer + "”" : said));
  }

  // forkPoll is the open vote over a fork's options, which the column's fork box draws.
  const forkPoll = (a) => (a && a.poll && /^the fork/.test(a.poll.for) ? a.poll : null);
  const inColumn = (box) => !!box.closest("#column");

  function draw() {
    const box = document.getElementById("audience");
    if (!box) return;
    const a = current;
    box.innerHTML = "";
    const forkVote = inColumn(box) && !!forkPoll(a) && !document.getElementById("candidates").classList.contains("hidden");
    box.classList.toggle("fork-vote", forkVote);
    const show = a && (a.prediction || (!forkVote && (a.poll || a.last_poll)));
    box.classList.toggle("hidden", !show);
    if (window.panelFit) window.panelFit();
    if (!show) return;
    if (a.prediction) box.appendChild(prediction(a.prediction));
    const q = a.poll || a.last_poll;
    if (q) {
      const b = poll(q);
      if (a.poll && a.last_poll && a.poll.status === "open") b.appendChild(lastLine(a.last_poll));
      box.appendChild(b);
    }
    if (window.panelFit) window.panelFit();
  }

  // In the full and lower-third layouts the box takes the moments' place beside the caption; the
  // column keeps it under the plan, where the runs board makes room.
  // panel.js sets the layout after this script runs.
  document.addEventListener("DOMContentLoaded", () => {
    if (!document.body.classList.contains("layout-full") && !document.body.classList.contains("layout-lower")) return;
    const lower = document.getElementById("lower"), box = document.getElementById("audience");
    if (lower && box) lower.insertBefore(box, document.getElementById("moments"));
  });

  window.renderAudience = (a) => { current = a || null; draw(); if (window.onAudience) window.onAudience(); };
  window.audienceForkPoll = () => forkPoll(current);
  window.audienceCurrent = () => current;
  window.audienceLeft = left;
  setInterval(() => {
    if (current && ((current.prediction && current.prediction.status === "active") || (current.poll && current.poll.status === "open"))) { draw(); if (window.onAudience) window.onAudience(); }
  }, 1000);
})();
