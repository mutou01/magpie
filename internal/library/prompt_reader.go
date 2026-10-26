package library

// A reader belongs to one read-only phase, never a Library lifetime or a
// request pair. Only shared source evidence is reused; file ownership, desired
// contents, disablement and per-template name matching are still checked live.
type promptReader struct {
	*Library
	native  map[string]promptNativeEvidence
	pi      map[string]promptPiEvidence
	sources map[promptScanSource][]promptDeclaration
}

type promptNativeEvidence struct {
	check nativePromptCheck
	err   error
}

type promptPiEvidence struct {
	snapshot         piCommandSnapshot
	check            piCommandCheck
	snapshotErr, err error
}

type promptScanSource struct {
	id string
	promptSource
}

type promptDeclaration struct {
	path, name, alias string
	err               error
}

func (r *promptReader) nativePromptEvidence(t PromptTarget) (nativePromptCheck, error) {
	k := nativePromptCacheKey(t)
	if e, ok := r.native[k]; ok {
		return e.check, e.err
	}
	c, err := r.Library.nativePromptEvidence(t)
	if r.native == nil {
		r.native = map[string]promptNativeEvidence{}
	}
	r.native[k] = promptNativeEvidence{c, err}
	return c, err
}
