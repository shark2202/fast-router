module fast-router

go 1.25.0

// C3 (Jev scorer) will require:
//   github.com/ebitengine/purego  (dlopen libllama without cgo, cross-compile)
// Added when C3 is implemented. C2/C4/C5 are pure Go, zero deps.

require github.com/ebitengine/purego v0.11.1 // indirect
