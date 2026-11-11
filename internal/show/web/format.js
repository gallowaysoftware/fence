// The run's stream format on the name card (docs/SHOW.md, "Formats"): title, handicaps, the lab's
// odds as a bar and the attempts so far. panel.js calls renderFormat with the player's format.
(() => {
  "use strict";
  const style = document.createElement("style");
  style.textContent = `
#format { margin-top: 10px; padding-top: 8px; border-top: 1px solid var(--line); }
#format .ft { display: flex; justify-content: space-between; align-items: baseline; gap: 8px; }
#format .fname { font-weight: 700; color: var(--money); letter-spacing: .02em; text-transform: uppercase; font-size: 16px; }
#format .fpct { font-size: 15px; color: var(--dim); white-space: nowrap; }
#format .fcard { font-size: 16px; color: var(--text); margin-top: 2px; }
#format .frules { display: flex; flex-wrap: wrap; gap: 4px; margin-top: 5px; }
#format .frule { font-size: 13px; padding: 1px 7px; border-radius: 9px; border: 1px solid var(--heat); color: var(--heat); }
#format .fodds { height: 5px; background: var(--line); border-radius: 3px; margin-top: 6px; overflow: hidden; }
#format .fodds > div { height: 100%; background: var(--accent); }
#format .fline { font-size: 14px; color: var(--dim); margin-top: 3px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }`;
  document.head.appendChild(style);

  function el(tag, cls, text) {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  }

  window.renderFormat = (f) => {
    const box = document.getElementById("format");
    if (!box) return;
    box.innerHTML = "";
    box.classList.toggle("hidden", !f);
    if (!f) return;
    const top = el("div", "ft");
    top.appendChild(el("span", "fname", f.title));
    top.appendChild(el("span", "fpct", f.lab_pct >= 0 ? "lab odds " + f.lab_pct + " %" : "untested"));
    box.appendChild(top);
    box.appendChild(el("div", "fcard", f.card));
    if (f.handicaps && f.handicaps.length) {
      const rules = el("div", "frules");
      for (const h of f.handicaps) rules.appendChild(el("span", "frule", h));
      box.appendChild(rules);
    }
    if (f.lab_pct >= 0) {
      const bar = el("div", "fodds"), fill = el("div");
      fill.style.width = Math.max(f.lab_pct, 1) + "%";
      bar.appendChild(fill);
      box.appendChild(bar);
    }
    box.appendChild(el("div", "fline", f.lab.charAt(0).toUpperCase() + f.lab.slice(1)));
    box.appendChild(el("div", "fline", "On stream: " + f.attempts));
  };
})();
