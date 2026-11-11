// The Twitch audience in the column (docs/SHOW.md, "Twitch"): the channel-points prediction and the
// chat vote, each with its question, split and result written out, so a VOD with no chat beside it
// still reads. panel.js calls renderAudience with an `audience` event's payload.
(() => {
  "use strict";
  const style = document.createElement("style");
  style.textContent = `
#audience { border-color: rgba(145, 70, 255, .45); padding: 9px 14px; flex: 0 0 auto; }
body:has(#audience:not(.hidden)) #ticker { min-height: 0; }
body:has(#audience:not(.hidden)) #board { display: none; }
body.layout-column:has(#audience:not(.hidden)) #plan-why { -webkit-line-clamp: 2; }
#lower:has(#audience:not(.hidden)) #moments { display: none; }
#lower #audience { align-self: stretch; overflow: hidden; background: var(--box); }
#audience .ab + .ab { margin-top: 7px; padding-top: 6px; border-top: 1px solid var(--line); }
#audience .ah { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
#audience .ak { font-size: 12px; letter-spacing: .14em; text-transform: uppercase; color: #b48cff; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
#audience .as { font-size: 13px; color: var(--dim); white-space: nowrap; font-variant-numeric: tabular-nums; }
#audience .aq { font-size: 17px; font-weight: 620; margin-top: 1px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
#audience .split { display: flex; height: 8px; border-radius: 4px; overflow: hidden; background: rgba(255,255,255,.07); margin-top: 5px; }
#audience .split > div { height: 100%; transition: width .6s; }
#audience .split .y { background: #9147ff; }
#audience .split .n { background: #5b6170; }
#audience .split .won { background: var(--good); }
#audience .sl { display: flex; justify-content: space-between; font-size: 14px; color: var(--dim); margin-top: 2px; font-variant-numeric: tabular-nums; }
#audience .sl .won { color: var(--good); }
#audience .opts { display: flex; gap: 6px; margin-top: 4px; }
#audience .opt { flex: 1 1 0; min-width: 0; font-size: 14px; padding: 2px 7px; border-radius: 6px; border: 1px solid var(--line); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-variant-numeric: tabular-nums; }
#audience .opt.top { border-color: #9147ff; color: var(--text); }
#audience .ar { font-size: 14px; margin-top: 3px; color: var(--dim); display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; }
#audience .ar.good { color: var(--good); }
#audience .ar.ans { color: var(--accent); }`;
  document.head.appendChild(style);

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
    const box = el("div", "ab");
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
    else if (p.claim && p.claim.replace(/[.?!]+$/, "") !== p.title.replace(/[.?!]+$/, "")) box.appendChild(el("div", "ar", "“" + p.claim + "”" + (p.by ? " · due day " + p.by : "")));
    if (p.skipped) box.appendChild(el("div", "ar", "Also staked, no prediction (" + p.skipped.why + "): “" + p.skipped.claim + "”"));
    return box;
  }

  function poll(q) {
    const box = el("div", "ab");
    box.appendChild(head("Chat vote · " + q.for, fmt(q.total) + " votes" + (q.status === "open" && q.closes_at ? " · " + left(q.closes_at) : " · closed")));
    box.appendChild(el("div", "aq", q.question));
    const opts = el("div", "opts");
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
    return el("div", q.answer ? "ar ans" : "ar", "Last vote (" + q.for + "): " + (q.answer ? "strategist: “" + q.answer + "”" : said));
  }

  function draw() {
    const box = document.getElementById("audience");
    if (!box) return;
    const a = current;
    box.innerHTML = "";
    const show = a && (a.prediction || a.poll || a.last_poll);
    box.classList.toggle("hidden", !show);
    if (!show) return;
    if (a.prediction) box.appendChild(prediction(a.prediction));
    const q = a.poll || a.last_poll;
    if (q) {
      const b = poll(q);
      if (a.poll && a.last_poll && a.poll.status === "open") b.appendChild(lastLine(a.last_poll));
      box.appendChild(b);
    }
  }

  // In the full and lower-third layouts the box takes the moments' place beside the caption; the
  // column keeps it under the plan, where the runs board makes room.
  // panel.js sets the layout after this script runs.
  document.addEventListener("DOMContentLoaded", () => {
    if (!document.body.classList.contains("layout-full") && !document.body.classList.contains("layout-lower")) return;
    const lower = document.getElementById("lower"), box = document.getElementById("audience");
    if (lower && box) lower.insertBefore(box, document.getElementById("moments"));
  });

  window.renderAudience = (a) => { current = a || null; draw(); };
  setInterval(() => {
    if (current && ((current.prediction && current.prediction.status === "active") || (current.poll && current.poll.status === "open"))) draw();
  }, 1000);
})();
