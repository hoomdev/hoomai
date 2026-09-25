"use strict";
/* Acciones de la tarjeta (spec acciones-desde-la-tarjeta, C3).
   Qué se puede hacer con cada tarjeta, adónde se puede soltar y por qué no
   lo calcula el binario (actions, drops y ghost en /api/board): esta página
   no tiene tabla de columnas ni de reglas, solo muestra y confirma. Todo POST
   de la cabina está acá y pasa por act( con el token, y ninguno lleva una
   columna: cada acción produce evidencia y la columna la recalcula boardcmd.
   Reusa de index.html: $, esc, toast, act y j; de tablero.js: tb, experto,
   marca, abrirDetalle y abrirTerminal. */

/* vocabulario normal */
const ACC_NORMAL = {
  guardar: "Guardar",
  integrar: "Integrar",
  guardarN: n => `Guardar ${n} cambio${n === 1 ? "" : "s"} de esta tarjeta.`,
  aprobar: (t, n, quien) => `Vas a aprobar el spec de "${t}" (${n} criterio${n === 1 ? "" : "s"}) con tu identidad: ${quien}. Si el spec cambia después, la aprobación deja de valer.`,
  integrarTexto: t => `Se cierra el trabajo de "${t}": se comprueba una última vez que todo esté verde y guardado, y la tarjeta pasa a Hecho. Juntarlo con el proyecto lo hacés vos después.`,
  cortado: "Hay un trabajo interrumpido en esta tarjeta: lo que dejó a medias también se guarda.",
  desdeComoQuedo: "El trabajo nuevo empieza desde el espacio de trabajo tal como quedó.",
  retoma: "Retoma la sesión que dejó el agente.",
  sinTope: nombre => `${nombre} no acepta un tope de presupuesto ni reporta costo: este trabajo no tiene tope en USD y su gasto no se descuenta del presupuesto de la tarjeta.`,
  minimo: (nombre, m) => `${nombre} necesita al menos ${m} USD por run.`,
  porLente: "En la revisión el presupuesto es por lente: puede usar hasta cuatro.",
  noSeDeshace: "No se puede deshacer.",
  empezo: rol => `El ${rol} empezó a trabajar: su fantasma aparece en la columna siguiente.`,
  sesionAbierta: "La sesión quedó abierta en el espacio de trabajo de la tarjeta.",
  sesionYaEstaba: "La sesión ya estaba abierta.",
  piloto: "Piloto automático: si este trabajo termina bien, la tarjeta sigue sola hasta la próxima columna tuya o hasta que se agote su presupuesto.",
};
/* fin vocabulario normal */

// etiquetas con acentos, por id de acción (el binario manda las suyas sin
// acentos en label)
const ACC_LABEL = {
  "pedir-arquitecto": "Pedir al arquitecto",
  "pedir-test-writer": "Pedir al test-writer",
  "pedir-writer": "Pedir al writer",
  "pedir-reviewer": "Pedir al reviewer",
  "aprobar": "Aprobar spec",
  "integrar": ACC_NORMAL.integrar,
  "reanudar": "Reanudar",
  "relanzar": "Volver a lanzar",
  "descartar": "Descartar cambios",
  "sesion": "Abrir sesión",
  "terminal": "Ver terminal",
};

function etiquetaAccion(a) {
  if (a.id === "guardar") return experto() ? "Guardar en git" : ACC_NORMAL.guardar;
  return ACC_LABEL[a.id] || a.label;
}

function nombreCLI(p) { return (MARCAS[p] && MARCAS[p].n) || p; }

function tarjetaDe(slug) {
  for (const col of (tb.board && tb.board.columns) || [])
    for (const c of col.cards) if (c.slug === slug) return c;
  return null;
}

/* ---------- la tarjeta: sus botones, y arrastrarla ---------- */

document.addEventListener("tablero:pintado", decorar);

function decorar() {
  for (const w of document.querySelectorAll("#tb-board .tcard-wrap")) {
    const c = tarjetaDe(w.dataset.slug);
    if (!c) continue;
    w.draggable = true; // soltarla en otra columna abre la misma acción, o dice por qué no
    const visibles = (c.actions || []).filter(a => !a.expert || experto());
    w.querySelector(".tacc").innerHTML = visibles.map(a =>
      `<button class="tact${a.enabled ? "" : " off"}" data-accion="${esc(a.id)}"${a.enabled ? "" : ` aria-disabled="true" title="${esc(a.why)}"`}>${esc(etiquetaAccion(a))}</button>`).join("");
  }
}

$("tb-board").addEventListener("click", e => {
  const b = e.target.closest(".tact");
  if (!b) return;
  const c = tarjetaDe(b.closest(".tcard-wrap").dataset.slug);
  const a = c && c.actions.find(x => x.id === b.dataset.accion);
  if (a) abrirAccion(c, a);
});

let arrastrada = null;

$("tb-board").addEventListener("dragstart", e => {
  const w = e.target.closest(".tcard-wrap");
  if (!w) return;
  arrastrada = w.dataset.slug;
  e.dataTransfer.setData("text/plain", arrastrada);
  e.dataTransfer.effectAllowed = "move";
});

$("tb-board").addEventListener("dragend", () => {
  arrastrada = null;
  for (const el of document.querySelectorAll("#tb-board .col")) el.classList.remove("dropok", "dropno");
});

$("tb-board").addEventListener("dragover", e => {
  const col = e.target.closest(".col");
  const c = arrastrada && tarjetaDe(arrastrada);
  if (!col || !c || col.dataset.col === c.column) return;
  e.preventDefault();
  const d = (c.drops || []).find(x => x.column === col.dataset.col);
  for (const el of document.querySelectorAll("#tb-board .col")) el.classList.remove("dropok", "dropno");
  col.classList.add(d && d.action ? "dropok" : "dropno");
});

$("tb-board").addEventListener("drop", e => {
  const col = e.target.closest(".col");
  const c = arrastrada && tarjetaDe(arrastrada);
  arrastrada = null;
  for (const el of document.querySelectorAll("#tb-board .col")) el.classList.remove("dropok", "dropno");
  if (!col || !c) return;
  e.preventDefault();
  if (col.dataset.col === c.column) return; // en su propia columna no pasa nada
  const d = (c.drops || []).find(x => x.column === col.dataset.col);
  if (!d) return;
  const a = d.action && c.actions.find(x => x.id === d.action);
  if (!a) {
    toast(d.why || c.plain, "err");
    return;
  }
  abrirAccion(c, a);
});

/* ---------- el diálogo: nada corre sin este sí ---------- */

let confirmarAccion = null;
// QUEDA: lo que devuelve un confirmar que ya cambió el diálogo por otro
const QUEDA = Symbol("queda");

function abrirAccion(c, a) {
  if (!a.enabled) {
    toast(a.why, "err");
    return;
  }
  if (a.id === "terminal") return abrirTerminal(c.slug);
  if (a.role) return dialogoRol(c, a); // el binario dice que rol lanza; la pagina no lo adivina
  switch (a.id) {
    case "aprobar": return dialogoAprobar(c, a);
    case "integrar": return dialogoIntegrar(c, a);
    case "guardar": return dialogoGuardar(c, a);
    case "descartar": return dialogoDescartar(c, a);
    case "sesion": return dialogoSesion(c, a);
  }
}

function dialogo(titulo, html, confirmar, textoOK) {
  $("acc-title").textContent = titulo;
  $("acc-body").innerHTML = html;
  $("acc-err").textContent = "";
  $("acc-ok").textContent = textoOK || "Confirmar";
  $("acc-ok").style.display = confirmar ? "" : "none";
  $("acc-ok").disabled = false;
  confirmarAccion = confirmar;
  $("accshade").style.display = "block";
  $("accdlg").style.display = "block";
}

function cerrarDialogo() {
  confirmarAccion = null;
  $("accshade").style.display = "none";
  $("accdlg").style.display = "none";
}

$("acc-cancel").addEventListener("click", cerrarDialogo);
$("accshade").addEventListener("click", cerrarDialogo);
document.addEventListener("keydown", e => {
  if (e.key === "Escape" && $("accdlg").style.display === "block") {
    e.stopImmediatePropagation();
    cerrarDialogo();
  }
}, true);

$("acc-ok").addEventListener("click", async () => {
  if (!confirmarAccion) return;
  $("acc-ok").disabled = true;
  $("acc-err").textContent = "";
  try {
    const msg = await confirmarAccion();
    if (msg === QUEDA) return;
    cerrarDialogo();
    if (msg) toast(msg, "ok");
  } catch (e) {
    // el error del servidor, tal como lo manda, y el diálogo queda abierto
    $("acc-err").textContent = e.message;
    $("acc-ok").disabled = false;
  }
});

function urlTarjeta(c) { return `/api/board/${encodeURIComponent(c.slug)}`; }

/* ---------- pedir al rol, reanudar, volver a lanzar ---------- */

function dialogoRol(c, a) {
  const ex = experto();
  const reanuda = a.id === "reanudar";
  const revision = a.role === "reviewer";
  const opciones = a.providers.map(p => `<option value="${esc(p.name)}"${p.ok ? "" : " disabled"}${p.default ? " selected" : ""}>
      ${esc(nombreCLI(p.name))}${p.ok ? "" : ` — ${esc(p.why)}`}</option>`).join("");
  const avisos = [];
  if (reanuda) avisos.push(ex ? `Retoma la sesión <code>${esc(a.resume_id)}</code>.` : esc(ACC_NORMAL.retoma));
  if ((reanuda || a.id === "relanzar") && c.unsynced.length) avisos.push(esc(ACC_NORMAL.desdeComoQuedo));
  // la cinta: que el encadenamiento sea consciente en el momento del si
  if (c.item.auto === "hasta-humano") avisos.push(esc(ACC_NORMAL.piloto));
  const html = `
    ${avisos.map(t => `<div class="aviso">${t}</div>`).join("")}
    <div class="fld"><label>Rol</label><div>${esc(a.role)}</div></div>
    <div class="fld"><label for="acc-prov">Provider</label>
      <select id="acc-prov"${reanuda ? " disabled" : ""}>${opciones}</select></div>
    <div class="fld"><label for="acc-model">Modelo</label>
      <input id="acc-model" placeholder="vacío: el que usa el CLI por defecto"></div>
    <div class="fld"><label for="acc-budget">Presupuesto en USD</label>
      <input id="acc-budget" type="number" min="0" step="0.1" value="${a.budget_usd != null ? esc(String(a.budget_usd)) : ""}" placeholder="vacío: sin tope"></div>
    <div class="aviso" id="acc-budgetnote"></div>
    ${revision ? "" : `<div class="fld"><label for="acc-pedido">Pedido</label><textarea id="acc-pedido">${esc(a.pedido)}</textarea></div>`}`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    const budget = $("acc-budget");
    const valor = budget.disabled || budget.value.trim() === "" ? null : Number(budget.value);
    const r = await act(urlTarjeta(c) + "/launch", {
      action: a.id,
      provider: $("acc-prov").value,
      model: $("acc-model").value.trim(),
      budget_usd: valor,
      pedido: revision ? "" : $("acc-pedido").value,
    });
    return ACC_NORMAL.empezo(r.role || a.role);
  }, "Lanzar");
  const alCambiar = () => {
    const p = a.providers.find(x => x.name === $("acc-prov").value);
    const budget = $("acc-budget");
    const notas = [];
    if (p && !p.budget) {
      budget.disabled = true;
      budget.value = "";
      notas.push(ACC_NORMAL.sinTope(nombreCLI(p.name)));
    } else {
      budget.disabled = false;
      if (budget.value === "" && a.budget_usd != null) budget.value = String(a.budget_usd);
      if (p && p.min_budget_usd) notas.push(ACC_NORMAL.minimo(nombreCLI(p.name), p.min_budget_usd));
      if (revision) notas.push(ACC_NORMAL.porLente);
    }
    $("acc-budgetnote").textContent = notas.join(" ");
  };
  $("acc-prov").addEventListener("change", alCambiar);
  alCambiar();
}

/* ---------- las firmas: aprobar e integrar ---------- */

function dialogoAprobar(c, a) {
  const html = `<p>${esc(ACC_NORMAL.aprobar(c.item.titulo, c.evidence.criteria, a.signer))}</p>
    <p><button class="act" id="acc-verspec">Leer el spec</button></p>`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    await act(`/api/specs/${encodeURIComponent(c.slug)}/approve`, { card: c.slug });
    return "Spec aprobado: la tarjeta sigue sola cuando su evidencia la gane.";
  }, "Aprobar");
  $("acc-verspec").addEventListener("click", () => {
    cerrarDialogo();
    abrirDetalle(c.slug);
  });
}

function dialogoIntegrar(c, a) {
  const html = `<p>${esc(ACC_NORMAL.integrarTexto(c.item.titulo))}</p>
    ${experto() ? `<p class="hint">Después, en la terminal: <code>git merge --no-ff hoom/${esc(c.slug)}</code></p>` : ""}`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    await act(`/api/tasks/${encodeURIComponent(c.slug)}/done`);
    return "Listo: la tarjeta pasa a Hecho.";
  }, "Integrar");
}

/* ---------- guardar y descartar ---------- */

function dialogoGuardar(c, a) {
  const ex = experto();
  const html = `
    ${c.interrupted ? `<div class="aviso">${esc(ACC_NORMAL.cortado)}</div>` : ""}
    <p>${esc(ACC_NORMAL.guardarN(a.paths.length))}</p>
    ${ex ? `<ul>${a.paths.map(p => `<li>${esc(p)}</li>`).join("")}</ul>
      <p class="hint">mensaje del commit: <code>hoom: guardar la tarjeta ${esc(c.slug)}</code></p>` : ""}`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    const r = await act(urlTarjeta(c) + "/save", { paths: a.paths });
    return ex ? `Guardado en ${r.commits.length} commit${r.commits.length === 1 ? "" : "s"}.` : "Guardado.";
  }, etiquetaAccion(a));
}

function dialogoDescartar(c, a) {
  const html = `<p>Vuelven a como estaban en el último commit, y se borran los archivos nuevos:</p>
    <ul>${a.paths.map(p => `<li>${esc(p)}</li>`).join("")}</ul>
    <div class="aviso">${esc(ACC_NORMAL.noSeDeshace)} La evidencia (.hoom/) no se toca.</div>`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    const r = await act(urlTarjeta(c) + "/discard", { paths: a.paths });
    return `Descartados: ${r.restored.length} restaurados, ${r.removed.length} borrados.`;
  }, "Descartar");
}

/* ---------- la sesión interactiva ---------- */

async function dialogoSesion(c, a) {
  let lista = [];
  try { lista = (await j("/api/providers")).filter(p => p.installed); } catch (e) { toast(e.message, "err"); return; }
  if (!lista.length) { toast("no hay ninguna CLI de IA instalada (mira hoom providers)", "err"); return; }
  const html = `<div class="fld"><label for="acc-sprov">CLI</label>
      <select id="acc-sprov">${lista.map(p => `<option value="${esc(p.name)}">${esc(nombreCLI(p.name))}</option>`).join("")}</select></div>
    <p class="hint">Abre la CLI interactiva en el espacio de trabajo de la tarjeta, con tmux, y la deja anotada en la tarjeta como quien escribe (declarado).</p>`;
  dialogo(`${etiquetaAccion(a)} · ${c.item.titulo}`, html, async () => {
    const r = await act(urlTarjeta(c) + "/session", { provider: $("acc-sprov").value });
    // queda abierto con lo que hace falta para entrar
    dialogo(`Sesión ${r.session}`, `<p>${esc(r.created ? ACC_NORMAL.sesionAbierta : ACC_NORMAL.sesionYaEstaba)}</p>
      <p>Para entrar desde una terminal: <code>${esc(r.attach)}</code></p>
      <p><button class="act" id="acc-verterm">Ver terminal</button></p>`, null);
    $("acc-verterm").addEventListener("click", () => {
      cerrarDialogo();
      abrirTerminal(c.slug);
    });
    return QUEDA; // el diálogo ya cambió: queda abierto
  }, "Abrir");
}
