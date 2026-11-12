// fence show panel: draws the show feed (docs/SHOW.md). Plain JS, no network beyond the
// show server itself.
(function () {
  "use strict";

  const params = new URLSearchParams(location.search);
  const path = location.pathname.replace(/\/+$/, "");
  const layout = params.get("layout") || (path.endsWith("/column") ? "column" : path.endsWith("/lower") ? "lower" : path.endsWith("/about") ? "about" : "full");
  const audioOn = params.has("audio") ? params.get("audio") !== "0" : layout !== "lower";
  document.body.classList.add("layout-" + layout);
  // The column has no room for moments; they sit next to the caption in the lower third.
  if (layout !== "column") document.getElementById("lower").appendChild(document.getElementById("moments"));
  else if (params.get("moments") !== "1") document.getElementById("moments").classList.add("hidden");
  if (params.get("snap") === "1") document.body.classList.add("snap");
  if (params.get("demo") === "1") document.body.classList.add("demo");

  const $ = (id) => document.getElementById(id);
  const TICKER = 7, MOMENTS = 3;
  const WEEKDAYS = ["", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
  let ticker = [], moments = [], day = 0, clock = null, nwPoints = [];
  // book is the route's lab band (player.book); ghost the latest morning against it.
  let book = null, ghost = null, prevRun = null;
  let playing = false, waiting = null, idleTimer = null;
  let player = null, hiddenUsed = 0, setup = [];
  // The "about this AI" card takes the ticker's place for ABOUT_SHOW ms at a run's start and every
  // ?about= seconds (default 480; 0: never). The /about layout shows it alone, always.
  const aboutEvery = params.has("about") ? Number(params.get("about")) * 1000 : 480000, ABOUT_SHOW = 22000;
  let aboutTimer = null;

  const fmt = (n) => (n == null ? "—" : Math.round(n).toLocaleString("en-US"));
  const signed = (n) => (n > 0 ? "+" : n < 0 ? "−" : "±") + fmt(Math.abs(n));
  const words = (s) => (s || "").replace(/_/g, " ");

  function text(el, s) { if (el.textContent !== s) el.textContent = s; }

  function tween(el, to) {
    const from = Number(el.dataset.v || to);
    el.dataset.v = to;
    if (from === to) { text(el, fmt(to)); return; }
    const t0 = performance.now(), dur = 600;
    function step(t) {
      const k = Math.min(1, (t - t0) / dur), e = 1 - Math.pow(1 - k, 3);
      text(el, fmt(from + (to - from) * e));
      if (k < 1 && el.dataset.v == to) requestAnimationFrame(step);
    }
    requestAnimationFrame(step);
  }

  function renderAbout() {
    const p = player || {};
    const who = [p.model || p.id, p.persona_name ? "in the role of " + p.persona_name : "", p.difficulty ? "on " + p.difficulty : ""].filter(Boolean).join(" ");
    text($("about-who"), who ? who + "." : "Waiting for a run.");
    text($("about-hidden"), hiddenUsed
      ? "Marks a move where the hands still read a hidden value: " + hiddenUsed + " this run. The goal is none."
      : "Would mark a move where the hands read a hidden value: none this run.");
    const ul = $("about-setup");
    ul.innerHTML = "";
    for (const l of setup) { const li = document.createElement("li"); li.textContent = l; ul.appendChild(li); }
  }

  function showAbout(ms) {
    if (layout === "about" || layout === "lower") return;
    renderAbout();
    $("about").classList.remove("hidden");
    $("ticker").classList.add("hidden");
    clearTimeout(aboutTimer);
    aboutTimer = setTimeout(() => { $("about").classList.add("hidden"); $("ticker").classList.remove("hidden"); }, ms);
  }
  if (layout === "about") $("about").classList.remove("hidden");
  else if (aboutEvery > 0 && params.get("snap") !== "1") setInterval(() => showAbout(ABOUT_SHOW), aboutEvery);

  function renderPlayer(p, run) {
    if (!p) return;
    player = p;
    book = p.book && p.book.p50 && p.book.p50.length ? p.book : null;
    prevRun = p.prev_run && p.prev_run.path && p.prev_run.path.length ? p.prev_run : null;
    renderAbout();
    text($("model"), p.model || p.id || "—");
    text($("persona"), p.persona_name ? "as " + p.persona_name : "no persona");
    text($("persona-tag"), [p.route ? words(p.route) + " route" : "", p.persona_tagline].filter(Boolean).join(" · "));
    text($("run-label"), (p.id || "") + (p.seed ? " · seed " + p.seed : ""));
    const chips = [p.difficulty, words(p.start), ...(p.perks || []).map(words), p.planner ? ({ plan: "LLM plans", hybrid: "LLM proposes, gym decides", local: "executor only" }[p.planner] || "planner: " + p.planner) : ""].filter(Boolean);
    $("chips").innerHTML = "";
    for (const c of chips) {
      const s = document.createElement("span");
      s.className = "chip"; s.textContent = c; $("chips").appendChild(s);
    }
    if (window.renderFormat) window.renderFormat(p.format);
    $("live-dot").classList.add("on");
  }

  function renderClock(c) {
    if (!c) return;
    clock = c;
    tween($("cash"), c.cash);
    text($("day"), String(day || "—") + (c.weekday ? " " + WEEKDAYS[c.weekday] : ""));
    const rent = $("rent");
    if (c.owned) {
      text($("rent-text"), "the shop is yours");
      text($("rent-days"), "");
    } else if (c.rent_due || c.mortgage_weekly) {
      // While the mortgage runs, its weekly payment falls on the rent day instead of the rent.
      const what = c.mortgage_weekly ? "payment " + fmt(c.mortgage_weekly) : "rent " + fmt(c.rent_due);
      text($("rent-text"), what + (c.arrears ? " + " + fmt(c.arrears) + " owed" : ""));
      text($("rent-days"), c.rent_days === 0 ? "due today" : "in " + c.rent_days + (c.rent_days === 1 ? " day" : " days"));
    }
    const owed = (c.mortgage_weekly || c.rent_due || 0) + (c.arrears || 0);
    rent.classList.toggle("short", !c.owned && owed > 0 && c.cash < owed);
    const pips = $("pips");
    if (pips.children.length !== 7) { pips.innerHTML = ""; for (let i = 0; i < 7; i++) { const d = document.createElement("div"); d.className = "pip"; pips.appendChild(d); } }
    for (let i = 0; i < 7; i++) pips.children[i].classList.toggle("on", !c.owned && i < Math.min(7, c.rent_days));
    const m = $("mortgage");
    if (c.mortgage_weekly) { m.classList.remove("hidden"); text(m, "mortgage " + fmt(c.mortgage_weekly) + " a week · " + c.mortgage_weeks_left + " weeks left"); }
    else if (c.offer_down && !c.owned) { m.classList.remove("hidden"); text(m, "mortgage offered: " + fmt(c.offer_down) + " down, " + fmt(c.offer_weekly) + " a week"); }
    else m.classList.add("hidden");
    if (c.net_worth) tween($("nw"), c.net_worth);
    if (c.buyout_price && !c.owned) {
      const pct = Math.max(0, Math.min(100, (100 * (c.net_worth || 0)) / c.buyout_price));
      $("buyout-fill").style.width = pct.toFixed(1) + "%";
      text($("buyout-text"), "buyout " + fmt(c.buyout_price) + " · " + pct.toFixed(0) + "% of it");
    } else if (c.owned) {
      $("buyout-fill").style.width = "100%";
      text($("buyout-text"), "bought out");
    }
  }

  function renderSpark(points) {
    nwPoints = points || [];
    $("spark").classList.toggle("book", !!book);
    if (book) { renderBook(); return; }
    $("spark-band").setAttribute("points", "");
    $("spark-median").setAttribute("points", "");
    $("spark-prev").setAttribute("points", "");
    const pts = nwPoints.slice(-120);
    if (pts.length < 2) { $("spark-line").setAttribute("points", ""); return; }
    let lo = Infinity, hi = -Infinity;
    for (const [, v] of pts) { lo = Math.min(lo, v); hi = Math.max(hi, v); }
    if (hi === lo) hi = lo + 1;
    const step = 500 / (pts.length - 1);
    $("spark-line").setAttribute("points", pts.map(([, v], i) => (i * step).toFixed(1) + "," + (40 - (36 * (v - lo)) / (hi - lo)).toFixed(1)).join(" "));
  }

  // With a book the line is the run's mornings against the lab band (10–90 %) and its median: the
  // last 60 days and three weeks ahead, so the viewer sees the race to come as well as the one run.
  function renderBook() {
    const path = (ghost && ghost.path) || [];
    const last = path.length ? path[path.length - 1][0] : 1;
    // The earlier run is drawn only as far as this one has got: a ghost, not a spoiler.
    const prev = prevRun ? prevRun.path.filter(([d]) => d <= last) : [];
    const to = Math.max(last, Math.min(book.p50.length, last + 21), 30), from = Math.max(1, to - 81);
    const at = (arr, d) => arr[Math.min(d, arr.length) - 1];
    let lo = Infinity, hi = -Infinity;
    for (let d = from; d <= to; d++) { lo = Math.min(lo, at(book.p10, d)); hi = Math.max(hi, at(book.p90, d)); }
    const shown = path.concat(prev).filter(([d]) => d >= from);
    for (const [, v] of shown) { lo = Math.min(lo, v); hi = Math.max(hi, v); }
    const pad = (hi - lo) * 0.06 + 1;
    lo -= pad; hi += pad;
    const x = (d) => ((500 * (d - from)) / (to - from)).toFixed(1);
    const y = (v) => (43 - (42 * (v - lo)) / (hi - lo)).toFixed(1);
    const line = (pts) => pts.filter(([d]) => d >= from).map(([d, v]) => x(d) + "," + y(v)).join(" ");
    const up = [], down = [], mid = [];
    for (let d = from; d <= to; d++) {
      up.push(x(d) + "," + y(at(book.p90, d)));
      down.push(x(d) + "," + y(at(book.p10, d)));
      mid.push(x(d) + "," + y(at(book.p50, d)));
    }
    $("spark-band").setAttribute("points", up.concat(down.reverse()).join(" "));
    $("spark-median").setAttribute("points", mid.join(" "));
    $("spark-line").setAttribute("points", line(path));
    $("spark-prev").setAttribute("points", line(prev));
    const bt = $("book-text");
    bt.classList.remove("hidden");
    const name = words(book.route) + " book";
    const prevNow = prev.length ? " · last run " + fmt(prev[prev.length - 1][1]) : "";
    text(bt, ghost ? name + " · median " + fmt(ghost.p50) + " · " + fmt(ghost.p10) + "–" + fmt(ghost.p90) + prevNow
      : name + (book.fallback ? " (no route)" : "") + " · " + book.wins + "/" + book.runs + " lab runs won, median day " + book.median_win_day);
  }

  function renderGhost(g, flash) {
    ghost = g || null;
    const el = $("pace");
    if (!ghost || !book) { el.classList.add("hidden"); $("book-text").classList.toggle("hidden", !book); if (book) renderBook(); return; }
    el.className = "pace " + ghost.status.replace(" ", "-");
    text(el, ghost.status);
    if (flash) { void el.offsetWidth; el.classList.add("flash"); }
    renderBook();
  }

  function renderHeat(h) {
    if (!h) return;
    const lvl = $("heat-level");
    if (!h.visible) {
      text(lvl, h.hot_items ? h.hot_items + " hot" : "clean");
      $("heat-fill").style.width = "0%";
      text($("heat-text"), h.hot_items ? h.hot_items + " stolen items on hand · no record yet" : "no security record yet");
      return;
    }
    text(lvl, h.level || String(h.evidence || 0));
    $("heat-fill").style.width = Math.min(100, h.evidence || 0) + "%";
    const bits = [];
    if (h.operation) bits.push(h.operation.toLowerCase() + (h.officer ? " · " + h.officer : ""));
    if (h.sentence) bits.push("sentence: " + h.sentence.toLowerCase());
    if (h.hot_items) bits.push(h.hot_items + " stolen on hand");
    text($("heat-text"), bits.join(" · ") || "record open, no case");
  }

  function renderPlan(ev, flash) {
    if (!ev || !ev.plan) return;
    const p = ev.plan;
    text($("plan-title"), "Plan · day " + p.day);
    text($("plan-source"), p.source === "llm" ? "model's plan" : p.source === "hybrid" ? "simulated choice" : p.source === "executor" ? "standing plan" : "model directives");
    const orders = (p.headline && p.headline.length ? p.headline : p.directives || []).slice(0, 5);
    const goals = (p.goals || []).slice(0, Math.max(0, 5 - orders.length));
    const ul = $("plan-lines");
    ul.innerHTML = "";
    if (!orders.length && !goals.length) { const li = document.createElement("li"); li.textContent = ev.text; ul.appendChild(li); }
    for (const l of orders) { const li = document.createElement("li"); li.textContent = l; ul.appendChild(li); }
    for (const l of goals) { const li = document.createElement("li"); li.className = "goal"; li.textContent = l; ul.appendChild(li); }
    const ch = (p.change && p.changes) ? p.changes.filter((c) => !c.startsWith("no longer")).slice(0, 3) : [];
    text($("plan-changes"), ch.length ? "changed: " + ch.join("; ") : "");
    const all = p.why || [];
    const reason = all.find((w) => !w.startsWith("overruled: "));
    const over = all.find((w) => w.startsWith("overruled: "));
    text($("plan-why"), [reason ? "why: " + reason : "", over ? over.replace(/^overruled: /, "bridge overruled: ") : ""].filter(Boolean).join(" · "));
    if (flash) { const el = $("plan"); el.classList.remove("flash"); void el.offsetWidth; el.classList.add("flash"); }
  }

  function renderCandidates(ev) {
    const box = $("candidates");
    if (!ev || !ev.candidates || !ev.candidates.options || !ev.candidates.options.length) { box.classList.add("hidden"); return; }
    box.classList.remove("hidden");
    text($("cand-metric"), ev.candidates.metric || "score");
    const t = $("cand-table");
    t.innerHTML = "";
    for (const o of ev.candidates.options.slice(0, 5)) {
      const tr = t.insertRow();
      if (o.chosen) tr.className = "chosen";
      tr.insertCell().textContent = (o.chosen ? "▸ " : "") + o.label;
      const s = tr.insertCell(); s.className = "n";
      // A fork the model decided has no score, only the choice.
      s.textContent = ev.candidates.metric === "choice" ? "" : (Math.round(o.score * 10) / 10) + (o.stderr ? " ±" + Math.round(o.stderr * 10) / 10 : "");
      const r = tr.insertCell(); r.className = "n"; r.textContent = o.rollouts ? o.rollouts + "×" : "";
    }
  }

  function li(ev, cls) {
    const el = document.createElement("li");
    el.className = cls || "";
    el.textContent = ev.text;
    // The autopilot's hands read a value no player sees: say so, without the value.
    if (ev.hidden_used && ev.hidden_used.length) {
      el.classList.add("hidden-used");
      el.title = "read a hidden value: " + ev.hidden_used.join(", ");
    }
    return el;
  }

  function pushTicker(ev, redraw) {
    ticker.push(ev); if (ticker.length > TICKER) ticker.shift();
    if (redraw === false) return;
    const ol = $("ticker-list");
    ol.appendChild(li(ev));
    while (ol.children.length > TICKER) ol.removeChild(ol.firstChild);
  }

  function pushMoment(ev, redraw) {
    moments.push(ev); if (moments.length > MOMENTS) moments.shift();
    // Between narrator lines the dimmed caption echoes the latest big moment.
    if (ev.level >= 2 && !playing && $("caption").classList.contains("idle")) text($("caption-text"), ev.text);
    if (redraw === false) return;
    const ol = $("moment-list");
    ol.insertBefore(li(ev, "k-" + ev.kind), ol.firstChild);
    while (ol.children.length > MOMENTS) ol.removeChild(ol.lastChild);
  }

  function renderBoard(rows) {
    const t = $("board-table");
    t.innerHTML = "";
    const recent = (rows || []).slice(-6).reverse();
    if (!recent.length) { $("board").classList.add("hidden"); return; }
    $("board").classList.remove("hidden");
    for (const r of recent) {
      const tr = t.insertRow();
      tr.className = r.live ? "live" : r.won ? "won" : "";
      tr.insertCell().textContent = (r.model || r.player) + (r.persona ? " · " + r.persona : "");
      const dif = tr.insertCell(); dif.className = "dim"; dif.textContent = r.difficulty || "";
      tr.insertCell().textContent = r.live ? "playing" : words(r.ending);
      const d = tr.insertCell(); d.className = "n"; d.textContent = "d" + r.days;
      const n = tr.insertCell(); n.className = "n"; n.textContent = r.net_worth ? fmt(r.net_worth) : "";
    }
  }

  // Narration: at most one line waits while another plays; a newer line replaces it.
  const voice = $("voice");

  function showCaption(line) {
    const c = $("caption");
    text($("caption-text"), line);
    c.classList.remove("idle");
    clearTimeout(idleTimer);
  }
  function idleLater(ms) {
    clearTimeout(idleTimer);
    idleTimer = setTimeout(() => $("caption").classList.add("idle"), ms);
  }

  function say(ev) {
    if (!ev.say) return;
    if (!audioOn || !ev.say.audio) {
      showCaption(ev.say.line);
      idleLater(Math.max(5000, ev.say.line.split(" ").length * 420));
      return;
    }
    if (playing) { waiting = ev; return; }
    play(ev);
  }

  function play(ev) {
    playing = true;
    showCaption(ev.say.line);
    $("caption").classList.add("speaking");
    voice.src = ev.say.audio;
    const p = voice.play();
    if (p && p.catch) p.catch(() => { $("unmute").classList.remove("hidden"); done(); });
  }

  function done() {
    playing = false;
    $("caption").classList.remove("speaking");
    idleLater(4000);
    if (waiting) { const w = waiting; waiting = null; play(w); }
  }
  voice.addEventListener("ended", done);
  voice.addEventListener("error", done);
  $("unmute").addEventListener("click", () => { $("unmute").classList.add("hidden"); voice.play().catch(() => {}); });

  function refreshBoard() {
    fetch("/state").then((r) => r.json()).then((s) => { renderBoard(s.board); renderSpark(s.net_worth); }).catch(() => {});
  }

  function applyState(s) {
    day = s.day || 0;
    hiddenUsed = s.hidden_used || 0;
    setup = s.setup || [];
    renderAbout();
    renderPlayer(s.player, s.run);
    if (s.end) $("live-dot").classList.remove("on");
    renderClock(s.clock);
    renderHeat(s.heat);
    renderPlan(s.plan, false);
    renderCandidates(s.candidates);
    renderGhost(s.ghost && s.ghost.ghost, false);
    renderSpark(s.net_worth);
    renderBoard(s.board);
    if (window.renderAudience) window.renderAudience(s.audience && s.audience.audience);
    ticker = []; moments = [];
    $("ticker-list").innerHTML = ""; $("moment-list").innerHTML = "";
    for (const e of s.ticker || []) pushTicker(e);
    for (const e of s.moments || []) pushMoment(e);
    if (s.say && s.say.say) { showCaption(s.say.say.line); idleLater(3000); }
  }

  function apply(ev) {
    if (ev.day > day) day = ev.day;
    if (ev.hidden_used && ev.hidden_used.length) { hiddenUsed++; renderAbout(); }
    if (ev.clock) renderClock(ev.clock);
    switch (ev.kind) {
      case "run_start":
        applyState({ day: ev.day, player: ev.player, run: ev.run, board: null, ticker: [], moments: [], setup: setup });
        refreshBoard();
        pushMoment(ev);
        showAbout(ABOUT_SHOW + 8000);
        return;
      case "plan": renderPlan(ev, ev.level >= 1); return;
      case "candidates": renderCandidates(ev); pushMoment(ev); return;
      case "heat": renderHeat(ev.heat); if (ev.level >= 1) pushMoment(ev); return;
      case "ghost": renderGhost(ev.ghost, ev.level >= 2); if (ev.level >= 2) pushMoment(ev); return;
      case "say": say(ev); return;
      case "audience": if (window.renderAudience) window.renderAudience(ev.audience); return;
      case "day": refreshBoard(); pushMoment(ev); return;
      case "run_end": $("live-dot").classList.remove("on"); refreshBoard(); pushMoment(ev); return;
    }
    if (ev.kind === "thought" || ev.level === 0) pushTicker(ev);
    else pushMoment(ev);
  }

  function connect() {
    // snap=1 draws once from /state and holds no connection open (headless screenshots).
    if (params.get("snap") === "1") {
      fetch("/state").then((r) => r.json()).then(applyState);
      return;
    }
    const es = new EventSource("/events");
    es.addEventListener("state", (m) => applyState(JSON.parse(m.data)));
    es.onmessage = (m) => apply(JSON.parse(m.data));
    es.onerror = () => { $("live-dot").classList.remove("on"); };
  }
  connect();
})();
