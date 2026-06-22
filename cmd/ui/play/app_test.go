package playui

import (
	"os"
	"strings"
	"testing"
)

func TestWorkspaceHistoryDrawerWiring(t *testing.T) {
	src, err := os.ReadFile("app.tsx")
	if err != nil {
		t.Fatalf("ReadFile(app.tsx): %v", err)
	}
	text := string(src)
	for _, want := range []string{
		`<TabsTrigger value="history">History</TabsTrigger>`,
		`<TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "chat" && "hidden")} value="chat">`,
		`<TabsContent forceMount className={cn("m-0 min-h-0 flex-1", workspaceTab !== "history" && "hidden")} value="history">`,
		`<WorkspaceHistoryPanel error={historyError} history={history} loading={loading} onPlay={playHistory} />`,
		`setWorkspaceTab("chat")`,
		`onHistoryChange={refreshActiveWorkspaceIntrospection}`,
		`getActiveWorkspaceHistory()`,
		`playActiveWorkspaceHistory(entry.id)`,
		`return fetchJSON<PageResponse<WorkspaceHistoryEntry>>("/peer-run/workspace/history")`,
		`"/peer-run/workspace/history/play"`,
		`JSON.stringify({ history_id: historyID })`,
		`onClick={() => void onPlay(entry)}`,
		`<Table className="table-fixed">`,
		`const replayable = entry.replay_available === true;`,
		`<span className="text-xs text-muted-foreground">-</span>`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("app.tsx missing workspace history wiring %q", want)
		}
	}
}
