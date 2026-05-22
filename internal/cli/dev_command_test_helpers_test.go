// purpose: Provide shared test fixtures for dev command test suites to avoid repeated inline config blocks.
// responsibilities: Build reusable config snippets used by multiple dev command tests.
// architecture notes: Helpers stay minimal and deterministic so tests remain explicit while reducing duplication noise.
package cli

func perTestStyleConfig() string {
	return testConfigCue() + `

devOutput: {
	color: "false"
	style: "per_test"
}
`
}
