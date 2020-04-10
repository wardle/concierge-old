package apiv1

// GetIdentifiersForSystem returns the identifier matching the system specified, it is exists
func (pt *Patient) GetIdentifiersForSystem(s string) ([]*Identifier, bool) {
	if pt == nil {
		return nil, false
	}
	result := make([]*Identifier, 0)
	for _, id := range pt.GetIdentifiers() {
		if id.GetSystem() == s {
			result = append(result, id)
		}
	}
	return result, len(result) > 0
}

// Match determines whether one patient is the same as another
func (pt *Patient) Match(other *Patient, identifierSystems []string) bool {
	if matchedIdentifiers(pt, other, identifierSystems) == false {
		return false
	}
	if pt.GetLastname() != other.GetLastname() {
		return false
	}
	if pt.GetBirthDate() != other.GetBirthDate() {
		return false
	}
	if pt.GetGender() != other.GetGender() {
		return false
	}
	return true
}

func matchedIdentifiers(pt1 *Patient, pt2 *Patient, systems []string) bool {
	for _, system := range systems {
		if matchedIdentifiersForSystem(pt1, pt2, system) == false {
			return false
		}
	}
	return true
}

// checks that at least one identifiers for a specified namespace matches
func matchedIdentifiersForSystem(pt1 *Patient, pt2 *Patient, system string) bool {
	if ids1, found := pt1.GetIdentifiersForSystem(system); found {
		if ids2, found := pt2.GetIdentifiersForSystem(system); found {
			for _, id1 := range ids1 {
				for _, id2 := range ids2 {
					if id1.GetValue() != id2.GetValue() {
						return true
					}
				}
			}
		}
	}
	return false
}
