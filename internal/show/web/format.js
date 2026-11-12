// The run's stream format on the name card (docs/SHOW.md, "Formats"): title, handicaps, the deadline
// countdown, the lab's odds as a bar and the attempts so far. panel.js calls renderFormat with the
// player's format and formatDay with each new day. Styles are in panel.css.
(() => {
  "use strict";
  let card = null, day = 0;

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  // The deadline in game days, so a viewer joining mid-run sees the stakes without the lab's words.
  function due() {
    const box = document.querySelector("#format .fdue");
    if (!box || !card || !card.deadline_day) return;
    const left = card.deadline_day - day;
    box.classList.toggle("late", left < 0);
    box.textContent = !day ? "deadline day " + card.deadline_day
      : left > 1 ? left + " days to day " + card.deadline_day
      : left === 1 ? "1 day left" : left === 0 ? "deadline today" : "past the deadline";
  }

  window.renderFormat = (f) => {
    const box = document.getElementById("format");
    if (!box) return;
    card = f || null;
    box.innerHTML = "";
    box.classList.toggle("hidden", !f);
    if (!f) return;
    const top = el("div", "ft");
    top.appendChild(el("span", "fname", f.title));
    if (f.lab_pct >= 0) {
      const bar = el("span", "fodds"), fill = el("span");
      fill.style.width = Math.max(f.lab_pct, 1) + "%";
      bar.appendChild(fill);
      top.appendChild(bar);
    }
    top.appendChild(el("span", "fpct", f.lab_pct >= 0 ? "lab odds " + f.lab_pct + " %" : "untested"));
    box.appendChild(top);
    box.appendChild(el("div", "fcard", f.card));
    if ((f.handicaps && f.handicaps.length) || f.deadline_day) {
      const rules = el("div", "frules");
      for (const h of f.handicaps || []) rules.appendChild(el("span", "frule", h));
      if (f.deadline_day) rules.appendChild(el("span", "fdue"));
      box.appendChild(rules);
    }
    box.appendChild(el("div", "fline flab", f.lab.charAt(0).toUpperCase() + f.lab.slice(1)));
    box.appendChild(el("div", "fline", "On stream: " + f.attempts));
    due();
  };
  window.formatDay = (d) => { if (d !== day) { day = d; due(); } };
})();
