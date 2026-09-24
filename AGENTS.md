
<!-- hoomai:contract -->
## Contrato hoomAI (verificacion obligatoria)

Este proyecto usa hoomAI. Reglas para CUALQUIER agente de codigo:

1. Antes de dar por terminado un cambio de codigo, ejecuta `hoom verify` y
   adjunta la ruta del veredicto (.hoom/verdicts/...) en tu respuesta final.
2. Veredicto ROJO = no se entrega. Corrige o reporta el bloqueo exacto.
3. Los roles y limites de cada agente estan en .hoom/agents/ y son obligatorios:
   un solo writer por tarea, test-writer NUNCA lee la implementacion,
   exploracion via scout, refactor legacy pasa por characterizer primero.
4. Nunca debilites, borres ni "ajustes" tests para hacer pasar un gate.
5. La narracion no cuenta: solo cuenta la evidencia del veredicto.
