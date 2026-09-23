"use strict";
/* Tablero: la cabina (specs tablero-de-solo-lectura, C2, y
   acciones-desde-la-tarjeta, C3). La columna de cada tarjeta, su medidor, su
   motivo, quién la trabajó, su fantasma y sus acciones los calcula el binario
   (boardcmd). Este archivo solo lee con j() y pinta: nada se envía y nada usa
   el token. Las acciones viven en acciones.js, que decora lo que se pinta acá
   cuando recibe el aviso "tablero:pintado".
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
  guardado: "guardado",
  estaComputadora: "esta computadora",
  sinHistoria: "Todavía no hay historia para esta tarjeta.",
  hoy: "Así está hoy",
  piloto: "piloto automático",
  problemas: "Lo que no cierra",
};
/* fin vocabulario normal */

// las fuentes de la historia, por el valor de source
const FUENTE_NORMAL = { git: NORMAL.guardado, telemetria: NORMAL.estaComputadora };
const FUENTE_EXPERTO = { git: "git", telemetria: "telemetría local" };

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
  term: { gen: 0 },   // el espejo de la terminal: sondeo mientras su sección está abierta
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
    // el fantasma: la tarjeta como va a quedar mientras un rol de su estación
    // trabaja; lo calcula el binario (ghost)
    const fantasmas = b.columns.flatMap(x => tb.filtro ? x.cards.filter(c => c.needs_decision) : x.cards)
      .filter(c => c.ghost && c.ghost.column === col.id);
    return `<section class="col${col.human ? " human" : ""}" data-col="${esc(col.id)}">
      <h3>${col.human ? "✋ " : ""}${esc(COLS[col.id] || col.name)}<span class="n">${n}</span></h3>
      <div class="cards">${cards.map(tarjeta).join("")}${fantasmas.map(fantasma).join("")}</div>
    </section>`;
  }).join("");
  for (const el of box.querySelectorAll(".col")) el.querySelector(".cards").scrollTop = scroll[el.dataset.col] || 0;
  if (!total) $("tb-warn").insertAdjacentHTML("beforeend", `<div class="empty">${esc(NORMAL.sinTarjetas)}</div>`);
  document.dispatchEvent(new Event("tablero:pintado"));
}

function fantasma(c) {
  const g = c.ghost;
  const paso = g.steps ? `paso ${g.step} de ${g.steps}: ${g.stage}` : g.stage;
  return `<div class="tghost" aria-hidden="true">
    <span class="t">${esc(c.item.titulo)}</span>
    <span>${marca(g.provider)} ${esc(g.role)} trabajando · ${esc(paso)}</span>
  </div>`;
}

function tarjeta(c) {
  const it = c.item;
  // .tacc queda vacío: lo llena acciones.js con lo que trae actions
  return `<div class="tcard-wrap" data-slug="${esc(c.slug)}"><button class="tcard${c.needs_decision ? " need" : ""}" data-slug="${esc(c.slug)}">
    <span class="t">${esc(it.titulo)}</span>
    <span class="meta">${esc(it.tipo)} · prioridad ${esc(it.prioridad)}${experto() ? ` · <code>${esc(c.slug)}</code>` : ""}${piloto(it)}${insignia(c)}</span>
    ${medidor(c)}
    ${subestados(c)}
    <span class="why">${motivo(c)}</span>
    <span class="foot">${gastoTarjeta(c.spend)}<span class="spacer"></span>${logos(c.providers)}</span>
  </button><div class="tacc"></div></div>`;
}

// el chip de la cinta: la tarjeta sigue sola entre dos firmas tuyas
function piloto(it) {
  return it.auto === "hasta-humano"
    ? ` <span class="tpilot" title="después de un trabajo que termina bien, la tarjeta sigue sola hasta la próxima columna tuya, un rojo o el fin de su presupuesto">${esc(NORMAL.piloto)}</span>` : "";
}

// la insignia del doctor: lo que no cierra en la evidencia de la tarjeta, con
// su acción exacta en modo experto (lo calcula el binario: doctor)
function insignia(c) {
  const ps = c.doctor || [];
  if (!ps.length) return "";
  const titulo = experto()
    ? ps.map(p => `${p.what}\nAccion: ${p.action}`).join("\n\n")
    : ps.map(p => p.plain).join("\n");
  return ` <span class="tdoc" title="${esc(titulo)}">⚕ ${ps.length}</span>`;
}

function listaProblemas(c, ex) {
  const ps = c.doctor || [];
  if (!ps.length) return "";
  return `<h3 class="dsub">${esc(NORMAL.problemas)}</h3><ul class="docl">${ps.map(p => ex
    ? `<li>${esc(p.what)}<br><span class="acc">Accion: ${esc(p.action)}</span> <small>(${esc(p.id)})</small></li>`
    : `<li>${esc(p.plain)}</li>`).join("")}</ul>`;
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
  const decl = (p && p.declared) || [];
  if (!p || (!p.writer && !p.reviewer && !decl.length)) return "";
  const out = [];
  if (p.writer) out.push(`<span title="escribió: ${esc(p.writer)}">✍ ${marca(p.writer)}</span>`);
  for (const d of decl.filter(d => d !== p.writer))
    out.push(`<span title="sesión interactiva abierta con ${esc(d)}: writer declarado, nadie lo vio escribir">✍? ${marca(d)}</span>`);
  if (p.reviewer) out.push(`<span title="revisó: ${esc(p.reviewer)}">🔎 ${marca(p.reviewer)}</span>`);
  if (p.cross === "no-cruzada") out.push(`<span class="warnmark" title="la misma CLI escribió y revisó (--same-provider)">misma CLI</span>`);
  if (p.cross === "cruzada-declarada") out.push(`<span class="warnmark" title="el writer solo está declarado por una sesión interactiva">cruzada (declarada)</span>`);
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
  olvidarHistoria();
  if (tb.pane === "historia") pedirHistoria();
  $("tdetail").classList.add("open");
  $("tshade").style.display = "block";
  pedirDetalle(++tb.genD);
}

function cerrarDetalle() {
  tb.open = null;
  tb.genD++;
  vivoPara(null);
  tb.term.gen++;
  olvidarHistoria();
  $("tb-terminal").open = false;
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
    for (const p of ["que", "quien", "pruebas", "historia"]) $("tb-p-" + p).classList.toggle("show", p === tb.pane);
    if (tb.pane === "historia" && hist.slug !== tb.open) pedirHistoria();
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
  $("tb-terminal").style.display = ex && c.actions.some(a => a.id === "terminal") ? "" : "none";
  vivoPara(c);
  document.dispatchEvent(new Event("tablero:pintado"));
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
    ${listaProblemas(d.card, ex)}
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

/* ---------- Ver terminal: el espejo de solo lectura del pane de tmux ---------- */

// abrirTerminal abre el detalle de la tarjeta en Quién con la sección
// Terminal abierta. Solo mira: la sección no tiene dónde escribir.
function abrirTerminal(slug) {
  if (tb.open !== slug) abrirDetalle(slug);
  $("tb-dtabs").querySelector('[data-pane="quien"]').click();
  $("tb-terminal").style.display = "";
  $("tb-terminal").open = true;
}

$("tb-terminal").addEventListener("toggle", () => {
  if ($("tb-terminal").open && tb.open) pedirTerminal(++tb.term.gen);
  else tb.term.gen++;
});

async function pedirTerminal(gen) {
  if (gen !== tb.term.gen || !tb.open || !$("tb-terminal").open || !visible()) return;
  const slug = tb.open;
  try {
    const t = await j(`/api/board/${encodeURIComponent(slug)}/terminal`);
    if (gen !== tb.term.gen) return;
    $("tb-termnote").textContent = t.available ? `sesión ${t.session} · solo lectura, se refresca cada segundo` : t.note;
    $("tb-term").innerHTML = t.available ? ansiHTML(t.text) : "";
  } catch (e) {
    if (gen !== tb.term.gen) return;
    $("tb-termnote").textContent = e.message;
    if (e.status === 404 || e.status === 400) return; // la tarjeta ya no existe: deja de pedir
  }
  setTimeout(() => pedirTerminal(gen), 1000);
}

// ansiHTML pinta lo que tmux ya pintó: los colores básicos, los brillantes y
// la negrita de las secuencias SGR pasan a <span>; las demás secuencias se
// descartan. El texto se escapa siempre.
const ANSI = ["#1d1f21", "#e06c75", "#98c379", "#e5c07b", "#61afef", "#c678dd", "#56b6c2", "#dcdfe4"];
const ANSI_BRILLO = ["#5c6370", "#ff7b86", "#b5e890", "#ffd580", "#7cc4ff", "#e19ef5", "#7fdbe6", "#ffffff"];
function ansiHTML(text) {
  const limpio = text.replace(/\x1b\][^\x07\x1b]*(\x07|\x1b\\)/g, "").replace(/\x1b\[[0-9;?]*[A-Za-ln-z]/g, "");
  let fg = null, bold = false, out = "";
  const abrir = () => (fg || bold) ? `<span style="${fg ? `color:${fg};` : ""}${bold ? "font-weight:700;" : ""}">` : "";
  let abierto = false;
  for (const parte of limpio.split(/(\x1b\[[0-9;]*m)/)) {
    const m = /^\x1b\[([0-9;]*)m$/.exec(parte);
    if (!m) {
      if (!parte) continue;
      if (!abierto && (fg || bold)) { out += abrir(); abierto = true; }
      out += esc(parte);
      continue;
    }
    if (abierto) { out += "</span>"; abierto = false; }
    for (const cod of (m[1] || "0").split(";").map(Number)) {
      if (cod === 0) { fg = null; bold = false; }
      else if (cod === 1) bold = true;
      else if (cod === 22) bold = false;
      else if (cod === 39) fg = null;
      else if (cod >= 30 && cod <= 37) fg = ANSI[cod - 30];
      else if (cod >= 90 && cod <= 97) fg = ANSI_BRILLO[cod - 90];
    }
  }
  if (abierto) out += "</span>";
  return out;
}

/* ---------- Historia: la tarjeta contada en el tiempo, y su replay ---------- */

// La historia sale de git y de la telemetría de esta computadora
// (/api/board/<slug>/timeline). Se pide al abrir la pestaña y con Actualizar:
// no sondea. El replay recorre las entradas y aplica al medidor los efectos
// que cada una trae (meter); el último cuadro es la tarjeta de hoy (card).
const hist = { slug: null, data: null, err: "", gen: 0, paso: -1, jugando: false, vel: 1, estado: {}, timer: 0 };

function olvidarHistoria() {
  pararReplay();
  hist.gen++;
  hist.slug = null;
  hist.data = null;
  hist.err = "";
  $("tb-historia").innerHTML = "";
}

async function pedirHistoria() {
  const slug = tb.open;
  if (!slug) return;
  const gen = ++hist.gen;
  pararReplay();
  hist.slug = slug;
  hist.paso = -1;
  $("tb-historia").innerHTML = `<div class="empty">cargando…</div>`;
  try {
    const d = await j(`/api/board/${encodeURIComponent(slug)}/timeline`);
    if (gen !== hist.gen) return;
    hist.data = d;
    hist.err = "";
  } catch (e) {
    if (gen !== hist.gen) return;
    hist.data = null;
    hist.err = e.message;
  }
  pintarHistoria();
}

function pararReplay() {
  hist.jugando = false;
  clearTimeout(hist.timer);
}

function estadoInicial(d) {
  const st = {};
  for (const m of d.meter) st[m.id] = "falta";
  return st;
}

function medidorReplay(d) {
  const n = d.meter.filter(m => /^CA-\d+$/.test(m.id)).length;
  let i = 0;
  return `<span class="meter">${d.meter.map(m => {
    const esCA = /^CA-\d+$/.test(m.id);
    if (esCA) i++;
    const nombre = esCA ? (experto() ? m.label : NORMAL.criterio(i, n)) : (experto() ? m.label : NORMAL.seg[m.id] || m.label);
    const st = hist.estado[m.id] || "falta";
    return `<i class="s-${esc(st)}" title="${esc(nombre)}"></i>`;
  }).join("")}</span>`;
}

function filaHistoria(e, i) {
  const ex = experto();
  const quien = ex ? e.who : (e.who || "").replace(/\s*<[^>]*>\s*$/, "");
  const fuente = ex ? FUENTE_EXPERTO[e.source] : FUENTE_NORMAL[e.source];
  let costo = "";
  if (e.cost_usd != null) costo = `${usd(e.cost_usd)} USD`;
  else if (e.kind === "sobre" || e.kind === "run") costo = "sin dato";
  if (e.tokens) costo += `${costo ? " · " : ""}${tokens(e.tokens)} tokens`;
  const clase = hist.paso < 0 ? "" : i === hist.paso ? " cur" : i > hist.paso ? " futuro" : "";
  return `<div class="hrow${clase}" data-i="${i}">
    <span class="w">${esc(when(e.at))}</span>
    <span>${esc(ex ? e.summary : e.plain)}
      <br><span class="w">${esc(quien)}${e.piloto ? ` · <span class="tpilot">${esc(NORMAL.piloto)}</span>` : ""}${ex && e.artifact ? ` · <code>${esc(e.artifact)}</code>` : ""}</span></span>
    <span><span class="src${e.source === "telemetria" ? " tel" : ""}">${esc(fuente || e.source)}</span><br><span class="c">${esc(costo)}</span></span>
  </div>`;
}

function pintarHistoria() {
  const box = $("tb-historia");
  const d = hist.data;
  if (!d) {
    box.innerHTML = hist.err ? `<div class="tberr">${esc(hist.err)}</div>` : "";
    return;
  }
  const hay = d.entries.length > 0;
  const fin = hay && hist.paso >= d.entries.length;
  const barra = `<div class="hbar">
    ${hay ? `<button class="act" data-h="play">${hist.jugando ? "Pausa" : "Reproducir"}</button>
    <button class="act" data-h="reset">Reiniciar</button>
    ${[1, 2, 4].map(v => `<button class="act${hist.vel === v ? " on" : ""}" data-h="vel" data-v="${v}">${v}x</button>`).join("")}` : ""}
    <span class="spacer"></span>
    <button class="act" data-h="update">Actualizar</button>
  </div>`;
  const notas = d.notes.length ? `<div class="hnotes">${d.notes.map(esc).join("<br>")}</div>` : "";
  const medidor = hist.paso >= 0 && !fin ? `<div class="hmeter">${medidorReplay(d)}</div>` : "";
  const hoy = fin || !hay ? `<div class="hoy"><h3 class="dsub" style="margin-top:0">${esc(NORMAL.hoy)}</h3>${medidor ? "" : ""}${medidorTarjeta(d.card)}<div class="why">${esc(d.card.plain)}</div></div>` : "";
  box.innerHTML = `${barra}${notas}${medidor}${hoy}
    ${hay ? `<div class="hlist">${d.entries.map(filaHistoria).join("")}</div>` : `<div class="empty">${esc(NORMAL.sinHistoria)}</div>`}`;
  const cur = box.querySelector(".hrow.cur");
  if (cur) cur.scrollIntoView({ block: "nearest" });
}

// el último cuadro: el medidor de la tarjeta de hoy, como en el tablero
function medidorTarjeta(c) { return medidor(c); }

function pasoReplay(gen) {
  if (gen !== hist.gen || !hist.jugando || !hist.data) return;
  const d = hist.data;
  hist.paso++;
  if (hist.paso < d.entries.length) {
    for (const fx of d.entries[hist.paso].meter || []) hist.estado[fx.id] = fx.state;
  } else {
    hist.jugando = false;
  }
  pintarHistoria();
  if (hist.jugando) hist.timer = setTimeout(() => pasoReplay(gen), 800 / hist.vel);
}

$("tb-historia").addEventListener("click", e => {
  const b = e.target.closest("button[data-h]");
  if (!b || !hist.data) {
    if (b && b.dataset.h === "update") pedirHistoria();
    return;
  }
  const d = hist.data;
  switch (b.dataset.h) {
    case "update":
      pedirHistoria();
      return;
    case "play":
      if (hist.jugando) {
        pararReplay();
      } else {
        if (hist.paso < 0 || hist.paso >= d.entries.length) {
          hist.paso = -1;
          hist.estado = estadoInicial(d);
        }
        hist.jugando = true;
        hist.timer = setTimeout(() => pasoReplay(hist.gen), 0);
      }
      break;
    case "reset":
      pararReplay();
      hist.paso = -1;
      hist.estado = estadoInicial(d);
      break;
    case "vel":
      hist.vel = +b.dataset.v || 1;
      break;
  }
  pintarHistoria();
});
