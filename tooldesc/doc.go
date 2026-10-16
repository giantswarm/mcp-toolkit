// Package tooldesc builds mcp-go tool descriptions that lead with a summary
// line, and checks a server's tools against description budgets.
//
// Behind muster, a model discovers tools through filter_tools and list_tools,
// which answer with each description's first line cut to [SummaryMax] runes.
// The model picks its tool from that summary; the rest of the description
// arrives with describe_tool, after the pick. [Describe] puts the summary on
// that first line and the details after a blank line, and panics on a summary
// that does not fit, so a server with one does not build.
//
// [Check] and [CheckServer] hold the rest of a server's descriptions to a
// [Budget] in its tests: the summary line, the whole description, each
// parameter's description, and a prefix no two tools may share (a shared
// boilerplate opening leaves every summary saying the same).
//
// What belongs in a summary and what in the details is in the toolkit's
// docs/conventions.md, "Tool descriptions".
package tooldesc
