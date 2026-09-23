"use strict";
/* Tablero: la cabina de solo lectura (spec tablero-de-solo-lectura, C2).
   La columna de cada tarjeta, su medidor, su motivo y quién la trabajó los
   calcula el binario (boardcmd). Esta pestaña solo lee con j() y pinta:
   nada se arrastra, nada se envía y nada usa el token. Las acciones son de C3.
   Reusa de index.html: $, esc, badge, stBadge, vBadge, when, j, mdRender,
   renderStage y appendFeed. */

/* vocabulario normal */
const NORMAL = {
  enEspacio: "en su espacio de trabajo",
  enProyecto: "en el proyecto",
  sinGuardar: n => `${n} cambio${n === 1 ? "" : "s"} sin guardar`,
  seg: { spec: "spec aprobado", build: "compila", static: "análisis estático", test: "pruebas", review: "revisión de 4 lentes" },
  criterio: (i, n) => `criterio ${i} de ${n}`,
  tuAceptacion: "aceptar e integrar",
  guardar: "guardar",
  sinTarjetas: "todavía no hay tarjetas: se crean con hoom item add \"<título>\"",
  ilegibles: n => `${n} tarjeta${n === 1 ? "" : "s"} no se pudo leer (el modo experto dice cuál${n === 1 ? "" : "es"})`,
  sinVeredicto: "todavía no hay una verificación de esta tarjeta",
};
/* fin vocabulario normal */

// dónde se leyó la evidencia, por el valor de evidence.source
const FUENTE = { worktree: NORMAL.enEspacio, arbol: NORMAL.enProyecto };

const COLS = {
  "backlog": "Backlog", "arquitecto": "Arquitecto", "tu-aprobacion": "Tu aprobación",
  "test-writer": "Test-writer", "writer": "Writer", "review": "Review",
  "tu-aceptacion": "Tu aceptación", "hecho": "Hecho",
};

// la marca de cada CLI: un glifo y su color, sin imágenes
const MARCAS = {
  claude:   { g: "✳",  fg: "#d97757", bg: "#2e211c", n: "Claude Code" },
  codex:    { g: ">_", fg: "#ffffff", bg: "#0d0d0d", n: "Codex" },
  gemini:   { g: "✦",  fg: "#6ea8fe", bg: "#18233a", n: "Gemini CLI" },
  opencode: { g: "oc", fg: "#f2c14e", bg: "#2b2616", n: "opencode" },
};

const tb = {
  on: false,          // la pestaña Tablero está a la vista
  modo: leerModo(),
  filtro: false,      // solo las que necesitan tu decisión (no se recuerda)
  board: null,        // el último tablero recibido
  err: "",
  pintado: "",        // lo último pintado: si no cambió, no se repinta
  genB: 0,
  open: null,         // slug del detalle abierto
  pane: "que",
  detail: null,
  derr: "",
  dpintado: "",
  genD: 0,
  diffOpen: false,
  vivo: { run: null, after: 0, gen: 0 },
};

function experto() { return tb.modo === "experto"; }
function visible() { return document.visibilityState === "visible"; }

function leerModo() {
  try { return localStorage.getItem("hoom-tablero-modo") === "experto" ? "experto" : "normal"; }
  catch (e) { return "normal"; }
}
function guardarModo(m) {
  try { localStorage.setItem("hoom-tablero-modo", m); } catch (e) {}
}

/* ---------- pestañas de la pantalla ---------- */

function mostrarPestana(cual) {
  tb.on = cual === "tablero";
  for (const b of document.querySelectorAll(".tabs .tab")) b.classList.toggle("on", b.dataset.tab === cual);
  $("tablero").style.display = tb.on ? "" : "none";
  $("cockpit").style.display = !tb.on && !runOpen ? "" : "none";
  $("runview").style.display = !tb.on && runOpen ? "" : "none";
  if (tb.on) arrancarTablero();
  else tb.genB++; // corta el sondeo del tablero
}
for (const b of document.querySelectorAll(".tabs .tab"))
  b.addEventListener("click", () => mostrarPestana(b.dataset.tab));

/* ---------- sondeo: 1 s después de cada respuesta, nunca dos a la vez ---------- */

function arrancarTablero() { pedirTablero(++tb.genB); }

async function pedirTablero(gen) {
  if (gen !== tb.genB || !tb.on || !visible()) return;
  try {
    tb.board = await j("/api/board");
    tb.err = "";
  } catch (e) {
    tb.err = e.message; // se conserva lo último pintado
  }
  if (gen !== tb.genB) return;
  pintarTablero();
  setTimeout(() => pedirTablero(gen), 1000);
}

document.addEventListener("visibilitychange", () => {
  if (!visible()) return;
  if (tb.on) arrancarTablero();
  if (tb.open) pedirDetalle(++tb.genD);
  if (tb.vivo.run) seguirVivo(++tb.vivo.gen);
});

/* ---------- el tablero ---------- */

function pintarTablero(forzar) {
  $("tb-err").textContent = tb.err ? `sin respuesta de hoom serve: ${tb.err} (reintento cada segundo)` : "";
  const b = tb.board;
  if (!b) return;
  const clave = JSON.stringify(b) + tb.modo + tb.filtro;
  if (!forzar && clave === tb.pintado) return;
  tb.pintado = clave;

  const w = b.warnings || [];
  $("tb-warn").innerHTML = !w.length ? "" : experto()
    ? `<div class="warnbox">! ${w.map(esc).join("<br>! ")}</div>`
    : `<div class="warnbox">${esc(NORMAL.ilegibles(w.length))}</div>`;

  const total = b.columns.reduce((n, c) => n + c.cards.length, 0);
  const tuyas = b.columns.reduce((n, c) => n + c.cards.filter(x => x.needs_decision).length, 0);
  $("tb-total").textContent = total ? `${total} tarjeta${total === 1 ? "" : "s"} · ${tuyas} necesita${tuyas === 1 ? "" : "n"} tu decisión` : "";

  const box = $("tb-board");
  const scroll = {};
  for (const el of box.querySelectorAll(".col")) scroll[el.dataset.col] = el.querySelector(".cards").scrollTop;
  box.innerHTML = b.columns.map(col => {
    const cards = tb.filtro ? col.cards.filter(c => c.needs_decision) : col.cards;
    const n = tb.filtro ? `${cards.length} de ${col.cards.length}` : `${col.cards.length}`;
    return `<section class="col${col.human ? " human" : ""}" data-col="${esc(col.id)}">
      <h3>${col.human ? "✋ " : ""}${esc(COLS[col.id] || col.name)}<span class="n">${n}</span></h3>
      <div class="cards">${cards.map(tarjeta).join("")}</div>
    </section>`;
  }).join("");
  for (const el of box.querySelectorAll(".col")) el.querySelector(".cards").scrollTop = scroll[el.dataset.col] || 0;
  if (!total) $("tb-warn").insertAdjacentHTML("beforeend", `<div class="empty">${esc(NORMAL.sinTarjetas)}</div>`);
}

function tarjeta(c) {
  const it = c.item;
  return `<button class="tcard${c.needs_decision ? " need" : ""}" data-slug="${esc(c.slug)}">
    <span class="t">${esc(it.titulo)}</span>
    <span class="meta">${esc(it.tipo)} · prioridad ${esc(it.prioridad)}${experto() ? ` · <code>${esc(c.slug)}</code>` : ""}</span>
    ${medidor(c)}
    ${subestados(c)}
    <span class="why">${motivo(c)}</span>
    <span class="foot">${gastoTarjeta(c.spend)}<span class="spacer"></span>${logos(c.providers)}</span>
  </button>`;
}

function motivo(c) {
  // si el rojo ya lo dijo con las mismas palabras, no se repite
  if (!experto()) return c.red && c.red.plain === c.plain ? "" : esc(c.plain);
  const mas = c.missing.length > 1 ? ` <small>(+${c.missing.length - 1})</small>` : "";
  return `${esc(c.missing[0] || "")}${mas}${c.next ? `<br><span class="next">siguiente: ${esc(c.next)}</span>` : ""}`;
}

function medidor(c) {
  const n = c.meter.filter(s => /^CA-\d+$/.test(s.id)).length;
  let i = 0;
  return `<span class="meter">${c.meter.map(s => {
    const esCA = /^CA-\d+$/.test(s.id);
    if (esCA) i++;
    const nombre = esCA ? (experto() ? s.label : NORMAL.criterio(i, n)) : (experto() ? s.label : NORMAL.seg[s.id] || s.label);
    return `<i class="s-${esc(s.state)}" title="${esc(nombre + ": " + s.detail)}"></i>`;
  }).join("")}</span>`;
}

function subestados(c) {
  const out = [];
  const r = c.running;
  if (r) {
    const quien = [r.role, r.provider].filter(Boolean).join(" · ");
    out.push(`<span class="s-run">⏵ en curso${r.envelope_id ? ` · paso ${r.step} de ${r.steps}: ${esc(r.stage)}` : ""}${quien ? ` (${esc(quien)})` : ""}</span>`);
  }
  const x = c.interrupted;
  if (x) out.push(`<span class="s-int">⏸ interrumpido en el paso ${x.step} de ${x.steps}${experto() ? ` (${esc(x.stage)})` : ""}</span>`);
  if (c.waiting_human) out.push(`<span class="s-hum">✋ esperando tu decisión</span>`);
  if (c.red) out.push(`<span class="s-red">✖ ${esc(experto() ? c.red.reason : c.red.plain)}</span>`);
  const u = c.unsynced.length;
  if (u) out.push(experto()
    ? `<span class="s-sync" title="${esc(c.unsynced.join("\n"))}">⇅ ${u} sin commitear</span>`
    : `<span class="s-sync">⇅ ${esc(NORMAL.sinGuardar(u))}</span>`);
  return out.length ? `<span class="subs">${out.join("")}</span>` : "";
}

function usd(v) { return (+v).toFixed(4).replace(/0+$/, "").replace(/\.$/, ""); }

function tokens(n) { return n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n); }

// gasto y presupuesto: lo que no se reportó se dice, nunca es un cero
function gastoTarjeta(s) {
  if (!s || (!s.runs && s.budget_usd == null)) return "";
  const tope = s.budget_usd != null ? s.budget_usd : null;
  let txt;
  if (s.cost_usd != null) txt = tope != null ? `${usd(s.cost_usd)} de ${usd(tope)} USD` : `${usd(s.cost_usd)} USD`;
  else txt = "sin dato de costo" + (tope != null ? ` · tope ${usd(tope)} USD` : "");
  const tok = (s.input_tokens || 0) + (s.output_tokens || 0);
  if (tok) txt += ` · ${tokens(tok)} tokens`;
  const pasado = s.cost_usd != null && tope != null && s.cost_usd > tope;
  return `<span class="${pasado ? "over" : ""}" title="telemetría local: lo que gastaron los runs de esta computadora">💲 ${esc(txt)}</span>`;
}

function marca(p) {
  const m = MARCAS[p] || { g: (p || "?").slice(0, 1).toUpperCase(), fg: "var(--text-2)", bg: "var(--panel)", n: p };
  return `<span class="pmark" style="color:${m.fg};background:${m.bg}" title="${esc(m.n)}">${esc(m.g)}</span>`;
}

function logos(p) {
  if (!p || (!p.writer && !p.reviewer)) return "";
  const out = [];
  if (p.writer) out.push(`<span title="escribió: ${esc(p.writer)}">✍ ${marca(p.writer)}</span>`);
  if (p.reviewer) out.push(`<span title="revisó: ${esc(p.reviewer)}">🔎 ${marca(p.reviewer)}</span>`);
  if (p.cross === "no-cruzada") out.push(`<span class="warnmark" title="la misma CLI escribió y revisó (--same-provider)">misma CLI</span>`);
  return out.join(" ");
}

/* ---------- barra: modo y filtro ---------- */

// el filtro deja solo las tarjetas con needs_decision (columnas moradas o
// interrumpidas), que calcula el binario
const FILTRO = "Necesitan tu decisión";
$("tb-filtro").textContent = FILTRO;

function pintarBarra() {
  for (const b of $("tb-modo").querySelectorAll("button")) b.classList.toggle("on", b.dataset.modo === tb.modo);
  $("tb-filtro").classList.toggle("on", tb.filtro);
  $("tb-filtro").setAttribute("aria-pressed", String(tb.filtro));
}
for (const b of $("tb-modo").querySelectorAll("button"))
  b.addEventListener("click", () => {
    tb.modo = b.dataset.modo === "experto" ? "experto" : "normal";
    guardarModo(tb.modo);
    pintarBarra();
    pintarTablero(true);
    if (tb.open) pintarDetalle(true);
  });
$("tb-filtro").addEventListener("click", () => {
  tb.filtro = !tb.filtro; // filtra por needs_decision, que calcula el binario
  pintarBarra();
  pintarTablero(true);
});
pintarBarra();

/* ---------- detalle: Qué, Quién, Pruebas ---------- */

$("tb-board").addEventListener("click", e => {
  const card = e.target.closest(".tcard");
  if (card) abrirDetalle(card.dataset.slug);
});

function abrirDetalle(slug) {
  tb.open = slug;
  tb.detail = null;
  tb.derr = "";
  tb.dpintado = "";
  $("tb-dtitle").textContent = slug;
  $("tb-dmeta").innerHTML = "";
  for (const id of ["tb-que", "tb-work", "tb-pruebas"]) $(id).innerHTML = `<div class="empty">cargando…</div>`;
  vivoPara(null);
  $("tdetail").classList.add("open");
  $("tshade").style.display = "block";
  pedirDetalle(++tb.genD);
}

function cerrarDetalle() {
  tb.open = null;
  tb.genD++;
  vivoPara(null);
  $("tdetail").classList.remove("open");
  $("tshade").style.display = "none";
}
$("tb-dclose").addEventListener("click", cerrarDetalle);
$("tshade").addEventListener("click", cerrarDetalle);
document.addEventListener("keydown", e => { if (e.key === "Escape" && tb.open) cerrarDetalle(); });

for (const b of $("tb-dtabs").querySelectorAll(".tab"))
  b.addEventListener("click", () => {
    tb.pane = b.dataset.pane;
    for (const x of $("tb-dtabs").querySelectorAll(".tab")) x.classList.toggle("on", x === b);
    for (const p of ["que", "quien", "pruebas"]) $("tb-p-" + p).classList.toggle("show", p === tb.pane);
  });

$("tb-diff").addEventListener("toggle", () => {
  tb.diffOpen = $("tb-diff").open;
  if (tb.diffOpen && tb.open) pedirDetalle(++tb.genD); // el diff se pide solo con su sección abierta
});

async function pedirDetalle(gen) {
  if (gen !== tb.genD || !tb.open || !visible()) return;
  const slug = tb.open;
  const conDiff = experto() && tb.diffOpen;
  try {
    tb.detail = await j(`/api/board/${encodeURIComponent(slug)}${conDiff ? "?diff=1" : ""}`);
    tb.derr = "";
  } catch (e) {
    if (gen !== tb.genD) return;
    if (e.status === 404 || e.status === 400) {
      tb.derr = `la tarjeta ya no existe: ${e.message}`;
      tb.detail = null;
      pintarDetalle(true);
      vivoPara(null);
      return; // deja de pedir
    }
    tb.derr = e.message;
  }
  if (gen !== tb.genD) return;
  pintarDetalle();
  setTimeout(() => pedirDetalle(gen), 1000);
}

function pintarDetalle(forzar) {
  const d = tb.detail;
  if (!d) {
    if (tb.derr) $("tb-dmeta").innerHTML = `<div class="tberr">${esc(tb.derr)}</div>`;
    return;
  }
  const clave = JSON.stringify(d) + tb.modo + tb.derr;
  if (!forzar && clave === tb.dpintado) return;
  tb.dpintado = clave;
  const c = d.card;
  const ex = experto();
  $("tb-dtitle").textContent = c.item.titulo;
  $("tb-dmeta").innerHTML = `
    <div class="meta" style="font-size:12px;color:var(--text-3)">${esc(c.item.tipo)} · prioridad ${esc(c.item.prioridad)} ·
      ${esc(COLS[c.column] || c.column_name)} · ${esc(ex ? `${c.evidence.source} ${c.evidence.dir}` : FUENTE[c.evidence.source] || "")}
      ${ex ? ` · <code>${esc(c.slug)}</code>` : ""}</div>
    ${medidor(c)}
    ${subestados(c)}
    ${ex
      ? `<ul>${c.missing.map(m => `<li>${esc(m)}</li>`).join("")}</ul>${c.next ? `<div class="next" style="font:12px var(--mono);color:var(--text-3)">siguiente: ${esc(c.next)}</div>` : ""}`
      : `<div class="why">${esc(c.plain)}</div>`}
    ${tb.derr ? `<div class="tberr">${esc(tb.derr)}</div>` : ""}`;
  $("tb-que").innerHTML = paneQue(d, ex);
  $("tb-diff").style.display = ex ? "" : "none";
  if (ex && tb.diffOpen) $("tb-diffbody").innerHTML = paneDiff(d.diff);
  $("tb-work").innerHTML = paneQuien(d, ex);
  $("tb-pruebas").innerHTML = panePruebas(d, ex);
  vivoPara(c);
}

// el enunciado de un criterio: texto escapado, con el codigo entre comillas
// invertidas como codigo
function enLinea(t) { return esc(t).replace(/`([^`]+)`/g, "<code>$1</code>"); }

function estadoCriterio(t) {
  if (t.traced_by === "comando") return `<span class="cmd">● con comando</span>`;
  if (t.traced_by === "test") return `<span class="ok">✓ con prueba</span>`;
  return `<span class="no">○ sin prueba</span>`;
}

function listaCriterios(d, ex, conArchivos) {
  if (!d.criteria.length) return `<div class="empty">${d.spec.exists ? "el spec todavía no está completo" : "todavía no hay spec"}</div>`;
  return `<ol class="crit">${d.criteria.map(t => `<li>${estadoCriterio(t)} ${t.text ? enLinea(t.text) : "<i>criterio citado en el spec</i>"}
    ${ex ? ` <small>${esc(t.id)}</small>` : ""}
    ${ex && conArchivos && t.files.length ? `<br><small>${t.files.map(esc).join(" · ")}</small>` : ""}</li>`).join("")}</ol>`;
}

function paneQue(d, ex) {
  const it = d.card.item;
  const a = d.spec.approval;
  return `
    ${it.pedido ? `<h3 class="dsub">Pedido</h3><div class="md"><p>${esc(it.pedido)}</p></div>` : ""}
    <h3 class="dsub">Criterios</h3>
    ${listaCriterios(d, ex, false)}
    ${ex ? `<div class="dmeta">spec <code>${esc(d.paths.spec || d.spec.path)}</code>${a ? ` · aprobado por <code>${esc(a.approved_by)}</code> el ${esc(when(a.approved_at))} · sha <code>${esc(a.sha256.slice(0, 8))}</code>` : " · sin aprobación vigente"}</div>` : ""}
    <h3 class="dsub">Spec</h3>
    ${d.spec.exists ? `<div class="md">${mdRender(d.spec.markdown)}</div>` : `<div class="empty">todavía no hay spec</div>`}`;
}

function paneDiff(df) {
  if (!df) return `<div class="empty">cargando…</div>`;
  if (!df.available) return `<div class="empty">${esc(df.note)}</div>`;
  return `<div class="dmeta">${esc(df.base)}...<code>${esc(df.head)}</code> · ${df.files.length} archivos · +${df.insertions} −${df.deletions}
      ${df.truncated ? ` · <span class="warnmark">recortado a 256 KiB</span>` : ""}</div>
    <table class="dtable">${df.files.map(f => `<tr><td class="g">${esc(f.path)}</td><td class="ms">+${f.insertions} −${f.deletions}</td></tr>`).join("")}</table>
    <pre class="tail">${esc(df.patch)}</pre>`;
}

function duracion(ms) {
  if (ms == null) return "—";
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${s % 60} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

function estadoTrabajo(w) {
  if (w.kind === "sobre") {
    switch (w.status) {
      case "en curso": return w.alive ? `trabajando · paso ${w.step} de ${w.steps} (${w.stage})` : `interrumpido en el paso ${w.step} de ${w.steps}`;
      case "entregable": return "entregó";
      case "no-entregable": return `no entregó (cortó en ${w.stage})`;
      case "sin-entrega": return "no dejó nada";
    }
    return w.status;
  }
  return { running: w.alive ? "corriendo" : "sin dueño vivo", done: "terminó", error: "error", canceled: "cancelado" }[w.status] || w.status;
}

function costoFila(u) {
  if (!u) return "—";
  const tok = (u.input_tokens || 0) + (u.output_tokens || 0);
  const partes = [u.cost_usd != null ? `${usd(u.cost_usd)} USD` : "sin dato de costo"];
  if (tok) partes.push(`${tokens(tok)} tokens`);
  return partes.join(" · ");
}

function paneQuien(d, ex) {
  const rows = d.work;
  const lista = !rows.length
    ? `<div class="empty">en esta computadora no corrió nada para esta tarjeta</div>`
    : `<table class="dtable">
        <tr><th>rol</th><th>CLI</th><th>estado</th><th>costo</th><th>duración</th></tr>
        ${rows.map(w => `<tr>
          <td>${esc(w.role || "sin rol")}${ex ? `<br><small style="color:var(--text-3)">${esc(w.kind)} <code>${esc(w.id)}</code>${w.run_id ? ` · run <code>${esc(w.run_id)}</code>` : ""}${w.isolated ? " · árbol ciego" : ""}</small>` : ""}</td>
          <td>${marca(w.provider)} ${esc(w.provider)}</td>
          <td>${esc(estadoTrabajo(w))}${ex && w.note ? `<br><small style="color:var(--text-3)">${esc(w.note)}</small>` : ""}</td>
          <td>${esc(costoFila(w.usage))}</td>
          <td class="ms">${esc(duracion(w.duration_ms))}</td>
        </tr>`).join("")}
      </table>`;
  return `<h3 class="dsub">Quién trabajó</h3>${lista}
    <div class="empty" style="font-style:italic">telemetría local: runs y sobres de esta computadora; no viajan</div>`;
}

function sevChip(s) { return badge((s || "").toUpperCase(), { high: "fail", medium: "error", low: "absent" }[s] || "skipped"); }

function panePruebas(d, ex) {
  const v = d.verdict;
  const veredicto = !v ? `<div class="empty">${esc(NORMAL.sinVeredicto)}</div>` : `
    <div class="dmeta">${vBadge(v.verdict)} ${esc(when(v.created_at))}
      ${ex ? `<br><code>${esc(v.id)}</code> · huella del veredicto <code>${esc(v.git?.change_fingerprint || "")}</code> · actual <code>${esc(d.fingerprint)}</code>${d.card.evidence.fingerprint_match ? " (coincide)" : " (distinta)"}` : ""}</div>
    <table class="dtable">
      <tr><th>estado</th><th>gate (* requerido)</th></tr>
      ${(v.gates || []).map(g => `<tr><td>${stBadge(g.status)}</td><td class="g">${esc(g.name)}${g.required ? " *" : ""}
        ${ex && (g.notes || g.output_tail) ? `${g.notes ? `<p class="tail-note">${esc(g.notes)}</p>` : ""}${g.output_tail ? `<pre class="tail">${esc(g.output_tail)}</pre>` : ""}` : ""}</td></tr>`).join("")}
    </table>`;
  const abiertos = d.findings.filter(f => f.status === "abierto");
  const resueltos = d.findings.filter(f => f.status !== "abierto");
  const hallazgo = f => `<tr><td>${sevChip(f.severity)}</td><td>${esc(f.status)}</td><td>${esc(f.description)}
      ${ex ? `<br><small style="color:var(--text-3)">${esc(f.lens || "")}${f.file ? " · " + esc(f.file) : ""} · ${esc(f.id)}</small>` : ""}
      ${f.resolution ? `<br><small style="color:var(--text-3)">evidencia: ${esc(f.resolution.evidence)}</small>` : ""}</td></tr>`;
  const hallazgos = !d.findings.length ? `<div class="empty">sin hallazgos de esta tarjeta</div>`
    : `<table class="dtable"><tr><th>sev</th><th>estado</th><th>hallazgo</th></tr>${[...abiertos, ...resueltos].map(hallazgo).join("")}</table>`;
  return `<h3 class="dsub">Último veredicto de la tarjeta</h3>${veredicto}
    <h3 class="dsub">Traza por criterio</h3>${listaCriterios(d, ex, true)}
    <h3 class="dsub">Hallazgos</h3>${hallazgos}`;
}

/* ---------- En vivo: el Escenario y el Feed del run activo ---------- */

// vivoPara sigue el run activo de la tarjeta. Sin tarjeta (abrir o cerrar el
// detalle) empieza de cero; cuando el run termina, queda su narración final.
function vivoPara(c) {
  const nota = t => { $("tb-livenote").textContent = t; };
  if (!c) {
    tb.vivo = { run: null, after: 0, gen: tb.vivo.gen + 1 };
    $("tb-stage").innerHTML = "";
    $("tb-feed").innerHTML = "";
    $("tb-livebox").style.display = "none";
    nota("");
    return;
  }
  const r = c.running;
  const id = r && r.run_id ? r.run_id : null;
  if (id && id !== tb.vivo.run) {
    tb.vivo = { run: id, after: 0, gen: tb.vivo.gen + 1 };
    $("tb-stage").innerHTML = "";
    $("tb-feed").innerHTML = "";
    $("tb-livebox").style.display = "";
    nota("");
    seguirVivo(tb.vivo.gen);
    return;
  }
  if (id) return; // el mismo run: su sondeo sigue
  const paso = r ? `paso ${r.step} de ${r.steps} (${r.stage}): en este paso no habla ningún agente` : "";
  if (tb.vivo.run) {
    if (r) nota(`${paso}; abajo, la narración del último run`);
    return; // queda la narración final del último run
  }
  nota(r ? paso : "nadie está trabajando en esta tarjeta ahora");
}

async function seguirVivo(gen) {
  const id = tb.vivo.run;
  if (!id || gen !== tb.vivo.gen || !visible()) return;
  try {
    const d = await j(`/api/runs/${encodeURIComponent(id)}?after=${tb.vivo.after}`);
    if (gen !== tb.vivo.gen) return;
    appendFeed(d.events, $("tb-feed"));
    tb.vivo.after = d.next;
    renderStage(await j(`/api/runs/${encodeURIComponent(id)}/stage`), $("tb-stage"));
    if (gen !== tb.vivo.gen) return;
    if (d.run.status === "running") setTimeout(() => seguirVivo(gen), 1000);
    else $("tb-livenote").textContent = "el run terminó: queda su narración final";
  } catch (e) {
    if (gen !== tb.vivo.gen) return;
    if (e.status === 404) {
      $("tb-livenote").textContent = "la narración de este run no está en este proyecto";
      return; // deja de pedir
    }
    $("tb-livenote").textContent = e.message;
    setTimeout(() => seguirVivo(gen), 1000);
  }
}
