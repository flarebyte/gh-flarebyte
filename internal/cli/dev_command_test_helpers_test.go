package cli

func perTestStyleConfig() string {
	return testConfigCue() + `

devOutput: {
	color: "false"
	style: "per_test"
}
`
}
