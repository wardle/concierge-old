package apiv1

// GetIdentifierForSystem returns the identifier matching the system specified, it is exists
func (pt *Patient) GetIdentifierForSystem(s string) (*Identifier, bool) {
	for _, id := range pt.GetIdentifiers() {
		if id.GetSystem() == s {
			return id, true
		}
	}
	return nil, false
}
