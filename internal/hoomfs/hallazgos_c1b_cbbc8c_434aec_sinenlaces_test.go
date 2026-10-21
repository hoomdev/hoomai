//go:build !windows

package hoomfs

// slErrSinEnlaces es hoomfs.ErrSinEnlaces, el error nuevo de la enmienda 2 de
// .hoom/specs/items-y-columna-derivada.md (CA-263, CA-265, CA-293). Esta en
// un archivo aparte para que, en un arbol sin el arreglo (903c6d8, codigo de
// la enmienda 1), esta sea la unica linea que no compila: reemplazandola por
// un errors.New cualquiera se ven las fallas de comportamiento de
// hallazgos_c1b_cbbc8c_434aec_test.go.
var slErrSinEnlaces = ErrSinEnlaces
