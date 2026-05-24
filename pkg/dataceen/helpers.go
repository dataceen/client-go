package dataceen

// Ptr returns a pointer to v. Useful for setting optional fields on
// generated *Update structs and other pointer-only inputs:
//
//	upd := &generated.CustomerUpdate{
//	    ExternalReference: dataceen.Ptr("new-ref"),
//	    IsActive:          dataceen.Ptr(true),
//	}
func Ptr[T any](v T) *T { return &v }
