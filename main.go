/*
Copyright © 2020 Eldrix Ltd and Mark Wardle <mark@wardle.org>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"github.com/wardle/concierge/cmd"
	_ "github.com/wardle/concierge/fhir"
	_ "github.com/wardle/concierge/england/sds"
)

// Version injected at build time
var version string

// Commit is last commit date/id injected at build time
var commit string

func main() {
	cmd.Version = version + ": " + commit
	cmd.Execute()
}
