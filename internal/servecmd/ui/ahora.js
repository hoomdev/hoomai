"use strict";
/* Ahora y Tarjeta (spec studio-tu-siguiente-paso): la portada del Studio y
   el detalle de una tarjeta. Quién tiene la pelota, el paso, las frases, el
   camino y el orden los resolvió el binario (boardcmd: card.pelota y
   board.pelota), el mismo que pinta hoom tablero: este archivo solo lee con
   j() y pinta. Ningún botón decide nada acá: todos van por abrirAccion de
   acciones.js, con su diálogo y su confirmación.
   Reusa de index.html: $, esc y j; de tablero.js: mostrarPestana, visible y
   experto; de acciones.js: abrirAccion. */

const ah = {
  on: false,       // Ahora está a la vista
  gen: 0,
  board: null,     // el último tablero recibido
  err: "",
  pintado: "",     // lo último pintado: si no cambió, no se repinta
  ficha: "",       // el slug de la Tarjeta a la vista ("" sin ella)
  genF: 0,
  det: null,       // su detalle (/api/board/<slug>)
  hist: null,      // su historia (/api/board/<slug>/timeline)
  ferr: "",
  no: false,       // la tarjeta no existe (404)
  pintadoF: "",
};

// las secciones de Ahora, una por grupo, en el orden de la lista
const SECCIONES_AH = [
  { grupo: "TE TOCA", titulo: n => `Te toca — ${n} ${n === 1 ? "decisión tuya" : "decisiones tuyas"}`, sub: () => "hoom no firma ni acepta por vos" },
  { grupo: "CORRIENDO", titulo: () => "En curso — lo que hoom hace ahora", sub: n => `${tarjetasAh(n)} trabajando` },
  { grupo: "DETENIDA", titulo: () => "Detenidas — hoom paró y dice por qué", sub: n => tarjetasAh(n) },
  { grupo: "EN COLA", titulo: () => "En cola — todavía no empezó", sub: n => tarjetasAh(n) },
  { grupo: "HECHO", titulo: () => "Hecho — lo que ya aceptaste", sub: n => tarjetasAh(n) },
];

// el badge de cada grupo que lo lleva (EN COLA y HECHO no llevan)
const BADGE_AH = {
  "TE TOCA": `<span class="ah-badge te-toca">te toca</span>`,
  "CORRIENDO": `<span class="ah-badge corriendo">CORRIENDO</span>`,
  "DETENIDA": `<span class="ah-badge detenida">DETENIDA</span>`,
};

/* ---------- el hash dice la pantalla ---------- */

// pantallaDelHash: vacío o #ahora, Ahora; #tablero y #cockpit, las de hoy;
// #tarjeta/<slug>, Tarjeta; cualquier otro, o un slug que no es válido,
// Ahora sin pedir nada.
function pantallaDelHash(h) {
  if (h === "#tablero" || h === "#cockpit") return { cual: h.slice(1), slug: "" };
  if (h.startsWith("#tarjeta/")) {
    const slug = h.slice("#tarjeta/".length);
    if (slugValido(slug)) return { cual: "tarjeta", slug };
  }
  return { cual: "ahora", slug: "" };
}

// slugValido tiene la forma de item.ValidSlug: minúsculas, números y
// guiones, sin guion al principio, hasta 64.
function slugValido(s) {
  return s.length <= 64 && /^[a-z0-9][a-z0-9-]*$/.test(s);
}

function irAlHash() {
  const p = pantallaDelHash(location.hash);
  mostrarPestana(p.cual, p.slug);
}

/* ---------- sondeo: 1 s después de cada respuesta, nunca dos a la vez ---------- */

// verAhora prende o apaga Ahora (lo llama mostrarPestana): apagada, su
// sondeo se corta.
function verAhora(on) {
  ah.on = on;
  if (on) pedirAhora(++ah.gen);
  else ah.gen++;
}

async function pedirAhora(gen) {
  if (gen !== ah.gen || !ah.on || !visible()) return;
  try {
    ah.board = await j("/api/board");
    ah.err = "";
  } catch (e) {
    ah.err = e.message; // se conserva lo último pintado y se reintenta
  }
  if (gen !== ah.gen) return;
  pintarAhora();
  setTimeout(() => pedirAhora(gen), 1000);
}

// verTarjeta muestra la Tarjeta de slug, o la apaga con "" (lo llama
// mostrarPestana): apagada, su sondeo se corta.
function verTarjeta(slug) {
  if (slug !== ah.ficha) {
    ah.det = null; ah.hist = null; ah.ferr = ""; ah.no = false; ah.pintadoF = "";
  }
  ah.ficha = slug;
  if (slug) {
    pintarFicha();
    pedirFicha(++ah.genF);
  } else ah.genF++;
}

// pedirFicha lee el detalle y la historia; un error de cualquiera de los dos
// (primero el del detalle) se dice y se reintenta. Una respuesta que llega
// tarde (otra generación u otra tarjeta) no toca ah.
async function pedirFicha(gen) {
  if (gen !== ah.genF || !ah.ficha || !visible()) return;
  const slug = ah.ficha;
  const base = "/api/board/" + encodeURIComponent(slug);
  const [d, h] = await Promise.allSettled([j(base), j(base + "/timeline")]);
  if (gen !== ah.genF || slug !== ah.ficha) return;
  const e = d.status === "rejected" ? d.reason : h.status === "rejected" ? h.reason : null;
  if (e) {
    ah.no = e.status === 404;
    ah.ferr = e.message;
  } else {
    ah.det = d.value;
    ah.hist = h.value;
    ah.ferr = "";
    ah.no = false;
  }
  pintarFicha();
  if (ah.no) return; // no existe: no hay nada que volver a pedir
  setTimeout(() => pedirFicha(gen), 1000);
}

document.addEventListener("visibilitychange", () => {
  if (!visible()) return;
  if (ah.on) pedirAhora(++ah.gen);
  if (ah.ficha && !ah.no) pedirFicha(++ah.genF);
});

/* ---------- los botones: todos por abrirAccion ---------- */

// cardAh es la tarjeta de slug tal como la trajo la lectura: la de la
// Tarjeta a la vista, o la del tablero.
function cardAh(slug) {
  if (ah.det && ah.det.card && ah.det.card.slug === slug) return ah.det.card;
  return ah.board ? indiceAh(ah.board)[slug] || null : null;
}

function alClicAh(ev) {
  const b = ev.target.closest("button[data-accion]");
  if (!b) return;
  const c = cardAh(b.dataset.slug);
  const a = c && (c.actions || []).find(x => x.id === b.dataset.accion);
  if (a) abrirAccion(c, a);
}
$("ahora").addEventListener("click", alClicAh);
$("tarjeta").addEventListener("click", alClicAh);

// botonPaso es el botón primario del paso de la tarjeta (paso.accion, con
// paso.etiqueta), o su texto en gris cuando no hay paso.
function botonPaso(c) {
  const p = c.pelota.paso;
  if (!p.accion) return `<span class="ah-gris">${esc(p.texto)}</span>`;
  return `<button class="ah-btn" data-slug="${esc(c.slug)}" data-accion="${esc(p.accion)}">${esc(p.etiqueta)}</button>`;
}

// accionesAh son las acciones habilitadas de la tarjeta que este modo
// muestra: las expert, solo en modo experto (CA-354, como acciones.js).
function accionesAh(c) {
  return (c.actions || []).filter(a => a.enabled && (!a.expert || experto()));
}

// botonAccion es un botón secundario de una acción de la tarjeta.
function botonAccion(c, a) {
  return `<button class="ah-btn2" data-slug="${esc(c.slug)}" data-accion="${esc(a.id)}">${esc(a.label)}</button>`;
}

function verLaTarjeta(c) {
  return `<a class="ah-btn2" href="#tarjeta/${esc(c.slug)}">Ver la tarjeta</a>`;
}

/* ---------- textos ---------- */

function tituloAh(c) {
  return (c.item && c.item.titulo) || c.slug;
}

function tarjetasAh(n) {
  return `${n} ${n === 1 ? "tarjeta" : "tarjetas"}`;
}

function dosAh(n) {
  return String(n).padStart(2, "0");
}

function horaAh(iso) {
  const d = new Date(iso);
  return isNaN(d) ? "" : `${dosAh(d.getHours())}:${dosAh(d.getMinutes())}`;
}

// cuandoAh es la hora de una entrada: HH:MM si es de hoy, DD/MM HH:MM si no.
function cuandoAh(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return "";
  const hoy = new Date();
  const dia = d.toDateString() === hoy.toDateString() ? "" : `${dosAh(d.getDate())}/${dosAh(d.getMonth() + 1)} `;
  return dia + horaAh(iso);
}

function fechaAh(iso) {
  const d = new Date(iso);
  return isNaN(d) ? "" : `${dosAh(d.getDate())}/${dosAh(d.getMonth() + 1)}/${d.getFullYear()}`;
}

// haceAh es el tiempo desde iso: segundos, minutos u horas y minutos; con
// iso en el futuro (reloj corrido), 0 min.
function haceAh(iso) {
  const s = Math.floor((Date.now() - new Date(iso)) / 1000);
  if (s < 0) return "0 min";
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min`;
  return `${Math.floor(m / 60)} h ${dosAh(m % 60)} min`;
}

function minutosAh(seg) {
  return `${Math.round(seg / 60)} min`;
}

function cortoAh(id) {
  return String(id || "").split("_").pop();
}

// motivoDetenidaAh es lo que la banda dice debajo de la frase de una
// DETENIDA: el trabajo que se cortó, o el plain de su rojo.
function motivoDetenidaAh(c) {
  if (c.interrupted) return `se cortó el trabajo del ${c.interrupted.role || "agente"}`;
  return c.red ? c.red.plain : c.plain;
}

// chipsAh: riesgo (cuando la evidencia trae riesgo, cualquiera) y el piloto
// encendido.
function chipsAh(c) {
  let out = "";
  const r = c.evidence && c.evidence.riesgo;
  if (r) out += ` <span class="ah-chip">riesgo ${esc(r)}</span>`;
  if (c.item && c.item.auto === "hasta-humano") out += ` <span class="ah-chip verde">piloto encendido</span>`;
  return out;
}

function pieAh() {
  return `<footer class="ah-pie">acá ves lo que hoom comprobó, no lo que el modelo cuenta</footer>`;
}

// indiceAh son las tarjetas del tablero por slug.
function indiceAh(b) {
  const out = {};
  for (const col of b.columns || []) for (const c of col.cards || []) out[c.slug] = c;
  return out;
}

/* ---------- Ahora ---------- */

function pintarAhora() {
  const b = ah.board;
  let html;
  if (!b) {
    html = ah.err ? `<p class="ah-err">no se pudo leer el tablero: ${esc(ah.err)}</p>` : `<p class="ah-gris">leyendo el tablero...</p>`;
  } else {
    const por = indiceAh(b);
    const orden = (b.pelota.orden || []).map(s => por[s]).filter(Boolean);
    const n = (b.warnings || []).length;
    html = (ah.err ? `<p class="ah-err">no se pudo leer el tablero: ${esc(ah.err)}</p>` : "") +
      (n ? `<p class="ah-aviso">${n} ${n === 1 ? "item" : "items"} no se pudieron leer: mirá el Tablero</p>` : "") +
      bandaAh(b, por, orden) + seccionesAh(orden);
  }
  html = `<div class="ah-pag">${html}${pieAh()}</div>`;
  if (html === ah.pintado) return;
  ah.pintado = html;
  $("ahora").innerHTML = html;
}

// bandaAh es TU SIGUIENTE PASO: la cuenta, y la tarjeta de caja o, sin
// ella, los textos del tablero v3.
function bandaAh(b, por, orden) {
  const pb = b.pelota, cu = pb.cuenta;
  const amarilla = cu.te_toca > 0 || cu.detenida > 0;
  const cab = `<div class="ah-banda-cab"><span>TU SIGUIENTE PASO</span>` +
    `<span class="ah-cuenta">te toca ${cu.te_toca} · corriendo ${cu.corriendo} · detenida ${cu.detenida}</span></div>`;
  const c = pb.caja ? por[pb.caja] : null;
  let cuerpo;
  if (c) {
    const g = c.pelota.grupo;
    const siguen = orden.filter(x => x !== c && (x.pelota.grupo === "TE TOCA" || x.pelota.grupo === "DETENIDA")).slice(0, 3);
    cuerpo = `<p class="ah-frase">${esc(c.pelota.frase)}</p>
      <p class="ah-gris">${esc(g === "DETENIDA" ? motivoDetenidaAh(c) : c.plain)}</p>
      <div class="ah-botones">${botonPaso(c)}${verLaTarjeta(c)}` +
      (siguen.length ? `<span class="ah-gris ah-corta">después: ${siguen.map(x => `«${esc(tituloAh(x))}»`).join(" · ")}</span>` : "") +
      `</div>`;
  } else {
    const corren = orden.filter(x => x.pelota.grupo === "CORRIENDO");
    const cola = orden.filter(x => x.pelota.grupo === "EN COLA");
    if (corren.length) {
      const p = corren[0], r = p.running;
      const que = r ? `${r.role || "agente"}, ${haceAh(r.started_at)}` : "piloto, decidiendo";
      const mas = corren.length > 1 ? ` y ${corren.length - 1} más` : "";
      cuerpo = `<p class="ah-frase">Nada te espera. hoom trabaja en ${tarjetasAh(corren.length)}.</p>
        <p class="ah-gris">corriendo: «${esc(tituloAh(p))}» (${esc(que)})${mas}</p>`;
    } else if (cola.length) {
      const e = cola[0];
      cuerpo = `<p class="ah-frase">Nada en curso. Podés empezar «${esc(tituloAh(e))}».</p>
        <div class="ah-botones">${botonPaso(e)}${verLaTarjeta(e)}</div>`;
    } else {
      cuerpo = `<p class="ah-frase">No hay nada pendiente.</p>
        <p class="ah-mono ah-gris">${esc('para sumar trabajo: hoom item add "<titulo>"')}</p>`;
    }
  }
  return `<section class="ah-banda${amarilla ? " amarilla" : ""}">${cab}${cuerpo}</section>`;
}

// seccionesAh: una por grupo con tarjetas, cada una en el orden de
// pelota.orden; Hecho, sus 5 primeras.
function seccionesAh(orden) {
  let out = "";
  for (const s of SECCIONES_AH) {
    const cs = orden.filter(c => c.pelota.grupo === s.grupo);
    if (!cs.length) continue;
    let cuerpo;
    switch (s.grupo) {
      case "TE TOCA": cuerpo = `<div class="ah-grilla">${cs.map(tarjetaTeToca).join("")}</div>`; break;
      case "CORRIENDO": cuerpo = cs.map(filaEnCurso).join(""); break;
      case "DETENIDA": cuerpo = `<div class="ah-grilla">${cs.map(tarjetaDetenidaAh).join("")}</div>`; break;
      case "HECHO":
        cuerpo = cs.slice(0, 5).map(c => lineaAh(c, true)).join("") +
          (cs.length > 5 ? `<p class="ah-gris">y ${cs.length - 5} más en el Tablero</p>` : "");
        break;
      default: cuerpo = cs.map(c => lineaAh(c, false)).join("");
    }
    out += `<section class="ah-sec"><div class="ah-sec-cab"><h2>${esc(s.titulo(cs.length))}</h2><p>${esc(s.sub(cs.length))}</p></div>${cuerpo}</section>`;
  }
  return out;
}

// pasoActual es la entrada actual del camino y su lugar (de 0).
function pasoActual(c) {
  const i = c.pelota.camino.findIndex(e => e.estado === "actual");
  return { i, e: i >= 0 ? c.pelota.camino[i] : null };
}

function tarjetaTeToca(c) {
  const { e } = pasoActual(c);
  const verde = c.evidence && c.evidence.verdict === "green" ? `<span class="ah-verde">VERDE</span> ` : "";
  const pend = c.pendientes && c.pendientes.texto ? `<p class="ah-gris">${esc(c.pendientes.texto)}</p>` : "";
  return `<article class="ah-tarj">
    <div><span class="ah-badge te-toca">${esc(e ? e.nombre.toUpperCase() : "")}</span> <span class="ah-mono ah-gris">${esc(c.slug)}</span></div>
    <h3>${esc(tituloAh(c))}</h3>
    <p>${verde}${esc(c.plain)}</p>${pend}
    <div class="ah-botones">${botonPaso(c)}${verLaTarjeta(c)}</div>
  </article>`;
}

function filaEnCurso(c) {
  const p = c.pelota, r = c.running;
  const { i, e } = pasoActual(c);
  const barra = p.camino.map(x => `<span class="${esc(x.estado)}"></span>`).join("");
  let quien = "";
  if (r) {
    quien = (r.provider ? esc(r.provider) + " · hace " : "hace ") + haceAh(r.started_at) +
      (r.timeout_s ? ` · tope ${minutosAh(r.timeout_s)}` : "");
  }
  return `<article class="ah-fila">
    <div class="ah-col">
      <div><a class="ah-titulo-link" href="#tarjeta/${esc(c.slug)}">${esc(tituloAh(c))}</a>${chipsAh(c)}</div>
      <div class="ah-barra">${barra}</div>
      <div class="ah-gris">${e ? `paso ${i + 1} de 8 — ${esc(e.nombre.toLowerCase())}` : ""}</div>
    </div>
    <div class="ah-col">
      <div class="ah-etq">AHORA</div>
      <p>${esc(p.ahora.frase)}</p>
      <p><span class="ah-badge corriendo">CORRIENDO</span> <span class="ah-mono ah-gris">${quien}</span></p>
    </div>
    <div class="ah-col">
      <div class="ah-etq">SIGUE</div>
      <p>${esc(p.sigue.frase)}</p>${p.sigue.gris ? `<p class="ah-mono ah-gris">${esc(p.sigue.gris)}</p>` : ""}
    </div>
  </article>`;
}

function tarjetaDetenidaAh(c) {
  const a = c.pelota.ahora;
  const acciones = accionesAh(c).slice(0, 3).map(x => botonAccion(c, x)).join("");
  return `<article class="ah-tarj">
    <div><span class="ah-badge detenida">DETENIDA</span> <span class="ah-mono ah-gris">${esc(c.slug)}</span></div>
    <h3>${esc(tituloAh(c))}</h3>
    <p>${esc(a.frase)}</p>${a.gris ? `<div class="ah-hundida">${esc(a.gris)}</div>` : ""}
    <div class="ah-botones">${acciones}${verLaTarjeta(c)}</div>
  </article>`;
}

// lineaAh es una fila de En cola o de Hecho; en Hecho, cuándo se aceptó. Su
// título largo se corta con … (ah-corta).
function lineaAh(c, hecha) {
  const cuando = hecha && c.item && c.item.hecho_en ? ` · aceptada ${esc(fechaAh(c.item.hecho_en))}` : "";
  return `<div class="ah-linea"><a class="ah-titulo-link ah-corta" href="#tarjeta/${esc(c.slug)}">${esc(tituloAh(c))}</a>
    <span class="ah-mono ah-gris">${esc(c.slug)}</span><span class="ah-gris">${esc(c.plain)}${cuando}</span></div>`;
}

/* ---------- Tarjeta ---------- */

function pintarFicha() {
  const volver = `<p><a class="ah-volver" href="#ahora">← Ahora</a></p>`;
  let html;
  if (ah.no) html = volver + `<p class="ah-err">no existe la tarjeta «${esc(ah.ficha)}»</p>`;
  else if (!ah.det) {
    html = volver + (ah.ferr ? `<p class="ah-err">no se pudo leer la tarjeta: ${esc(ah.ferr)}</p>` : `<p class="ah-gris">leyendo la tarjeta...</p>`);
  } else {
    html = (ah.ferr ? `<p class="ah-err">no se pudo leer la tarjeta: ${esc(ah.ferr)}</p>` : "") + fichaAh(ah.det, ah.hist);
  }
  html = `<div class="ah-pag">${html}${pieAh()}</div>`;
  if (html === ah.pintadoF) return;
  ah.pintadoF = html;
  $("tarjeta").innerHTML = html;
}

function fichaAh(d, h) {
  const c = d.card, p = c.pelota;
  return `<p><a class="ah-volver" href="#ahora">← Ahora</a></p>
  <div class="tj-cab">
    <h1>${esc(tituloAh(c))}</h1>
    <div class="tj-chips"><span class="ah-mono ah-gris">${esc(c.slug)}</span>${chipsAh(c)} ${BADGE_AH[p.grupo] || ""}</div>
  </div>
  <section class="tj-panel"><h2>Camino — de tu firma a tu aceptación</h2><div class="tj-camino">${caminoFicha(c)}</div></section>
  <div class="tj-cols">
    <div class="tj-col">${ahoraFicha(c)}${sigueFicha(c)}${historiaFicha(h)}</div>
    <div class="tj-col">${topesFicha(c)}${evidenciaFicha(d)}${botonesFicha(c)}</div>
  </div>`;
}

// caminoFicha son las 8 entradas de pelota.camino: su barra, su nombre y su
// estado en palabras (listo, el texto de la actual, falta).
function caminoFicha(c) {
  const g = { "TE TOCA": "te-toca", "CORRIENDO": "corriendo", "DETENIDA": "detenida" }[c.pelota.grupo] || "";
  const palabra = { hecho: "listo", falta: "falta" };
  return `<ol>${c.pelota.camino.map(e => {
    const actual = e.estado === "actual";
    const dice = actual ? e.texto : palabra[e.estado] || "";
    return `<li class="${esc(e.estado)}${actual ? " " + g : ""}"${actual ? ` aria-current="step"` : ""}>` +
      `<span class="barra"></span><span class="nombre">${esc(e.nombre)}</span><span class="estado">${esc(dice)}</span></li>`;
  }).join("")}</ol>`;
}

function ahoraFicha(c) {
  const a = c.pelota.ahora, r = c.running;
  let corre = "";
  if (r) {
    const prov = r.provider ? esc(r.provider) + " · " : "";
    corre = `<p><span class="ah-badge corriendo">CORRIENDO</span> <span class="ah-mono ah-gris">${prov}empezó ${horaAh(r.started_at)} · hace ${haceAh(r.started_at)}</span></p>`;
    if (r.timeout_s) {
      const seg = Math.max(0, (Date.now() - new Date(r.started_at)) / 1000);
      const pct = Math.min(100, seg / r.timeout_s * 100);
      corre += `<p>tiempo de este trabajo <span class="ah-mono">${Math.floor(seg / 60)} de ${Math.round(r.timeout_s / 60)} min</span></p>` +
        `<div class="tj-barra"><span style="width:${pct.toFixed(0)}%"></span></div>`;
    }
    if (r.envelope_id) corre += `<p class="ah-mono ah-gris">sobre ${esc(r.envelope_id)}</p>`;
  }
  return `<section class="tj-panel${r ? " abierto" : ""}"><h2>AHORA — QUÉ ESTÁ HACIENDO</h2>
    <p class="ah-frase">${esc(a.frase)}</p>${a.gris ? `<p class="ah-gris">${esc(a.gris)}</p>` : ""}${corre}</section>`;
}

function sigueFicha(c) {
  const s = c.pelota.sigue;
  return `<section class="tj-panel"><h2>SIGUE — QUÉ PASA DESPUÉS</h2>
    <p>${esc(s.frase)}</p>${s.gris ? `<p class="ah-mono ah-gris">${esc(s.gris)}</p>` : ""}</section>`;
}

// historiaFicha: de la entrada más nueva a la más vieja, hasta 20.
function historiaFicha(h) {
  const es = ((h && h.entries) || []).slice().sort((x, y) => new Date(y.at) - new Date(x.at)).slice(0, 20);
  const filas = es.map(e => {
    const extra = [e.provider, e.ref].filter(Boolean).join(" · ");
    return `<div><span class="ah-mono ah-gris">${esc(cuandoAh(e.at))}</span><span>${esc(e.plain)}` +
      (extra ? ` <span class="ah-mono ah-gris">${esc(extra)}</span>` : "") + `</span></div>`;
  }).join("");
  return `<section class="tj-panel"><h2>Historia — paso a paso</h2>` +
    (filas ? `<div class="tj-hist">${filas}</div>` : `<p class="ah-gris">Todavía no hay historia para esta tarjeta.</p>`) + `</section>`;
}

// topesFicha: solo con presupuesto; sin costo reportado, sin barra.
function topesFicha(c) {
  const s = c.spend || {};
  if (!(s.budget_usd > 0)) return "";
  let fila;
  if (s.cost_usd == null) {
    fila = `<p>presupuesto <span class="ah-mono">sin dato de costo · tope ${s.budget_usd.toFixed(2)} USD</span></p>`;
  } else {
    const pct = Math.min(100, s.cost_usd / s.budget_usd * 100);
    fila = `<p>presupuesto <span class="ah-mono">${s.cost_usd.toFixed(2)} de ${s.budget_usd.toFixed(2)} USD</span></p>` +
      `<div class="tj-barra tj-gasto${pct >= 80 ? " alto" : ""}"><span style="width:${pct.toFixed(0)}%"></span></div>`;
  }
  return `<section class="tj-panel"><h2>Topes — hasta dónde llega solo</h2>${fila}</section>`;
}

// evidenciaFicha: el veredicto del detalle, gate por gate; el fail de un
// rojo esperado (red.esperado) se dice FAIL esperado.
function evidenciaFicha(d) {
  const c = d.card, v = d.verdict;
  let cuerpo;
  if (!v) cuerpo = `<p class="ah-gris">todavía no hay veredicto</p>`;
  else {
    const esperado = !!(c.red && c.red.esperado);
    const gates = (v.gates || []).map(g => {
      const st = String(g.status || "");
      let cls = "otro", dice = st.toUpperCase();
      if (st === "pass") cls = "pass";
      else if (st === "fail" && esperado) { cls = "esperado"; dice = "FAIL esperado"; }
      else if (st === "fail" || st === "error") cls = "fail";
      return `<div class="tj-gate"><span>${esc(g.name)}</span><span class="${cls}">${esc(dice)}</span></div>`;
    }).join("");
    const bloquean = ((c.evidence && c.evidence.blocking_findings) || []).length;
    const pend = c.pendientes && c.pendientes.texto ? c.pendientes.texto : "sin pendientes";
    cuerpo = `<p class="ah-mono">veredicto ${esc(cortoAh(v.id))} · ${v.verdict === "green" ? "VERDE" : "ROJO"}</p>${gates}
      <p>hallazgos que bloquean: ${bloquean}</p><p class="ah-gris">${esc(pend)}</p>`;
  }
  return `<section class="tj-panel"><h2>Evidencia — lo último comprobado</h2>${cuerpo}</section>`;
}

// botonesFicha: el primario del paso y un secundario por cada otra acción
// habilitada que este modo muestra, en su orden; con el piloto encendido,
// cómo apagarlo.
function botonesFicha(c) {
  const p = c.pelota.paso;
  const otras = accionesAh(c).filter(a => a.id !== p.accion).map(a => botonAccion(c, a)).join("");
  const piloto = c.item && c.item.auto === "hasta-humano"
    ? `<p class="ah-mono ah-gris">para apagar el piloto: hoom item set ${esc(c.slug)} --auto no</p>` : "";
  return `<section class="tj-panel"><div class="ah-botones">${botonPaso(c)}${otras}</div>${piloto}</section>`;
}

/* ---------- arranque ---------- */

window.addEventListener("hashchange", irAlHash);
irAlHash();
