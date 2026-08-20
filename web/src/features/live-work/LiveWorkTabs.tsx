import { useSearchParams } from "react-router-dom";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/Tabs";

import { CodeActivityStatus } from "./CodeActivityStatus";
import { HandoffList } from "./HandoffList";
import { IntentTable } from "./IntentTable";

type LiveWorkView = "intents" | "code" | "handoffs";
type RecordItem = Record<string, unknown>;

function isView(value: string | null): value is LiveWorkView {
  return value === "intents" || value === "code" || value === "handoffs";
}

export function LiveWorkTabs({
  intents,
  codeActivity,
  handoffs,
}: {
  intents: RecordItem[];
  codeActivity?: RecordItem;
  handoffs: RecordItem[];
}) {
  const [search, setSearch] = useSearchParams();
  const requestedView = search.get("view");
  const view: LiveWorkView = isView(requestedView) ? requestedView : "intents";

  const selectView = (nextView: string) => {
    if (!isView(nextView)) return;
    const nextSearch = new URLSearchParams(search);
    if (nextView === "intents") nextSearch.delete("view");
    else nextSearch.set("view", nextView);
    setSearch(nextSearch, { replace: true });
  };

  return (
    <Tabs value={view} onValueChange={selectView}>
      <div className="live-work-tabs">
        <TabsList className="live-work-tabs-list">
          <TabsTrigger value="intents">Intents y scopes <span>{intents.length}</span></TabsTrigger>
          <TabsTrigger value="code">Código en vivo</TabsTrigger>
          <TabsTrigger value="handoffs">Handoffs <span>{handoffs.length}</span></TabsTrigger>
        </TabsList>

        <TabsContent value="intents" className="live-work-tab-panel live-work-intents">
          <header className="live-work-tab-heading">
            <span><p className="pact-kicker">COORDINACIÓN</p><h2>Intents y scopes</h2></span>
            <small>{intents.length} {intents.length === 1 ? "intent declarado" : "intents declarados"}</small>
          </header>
          <IntentTable items={intents} />
        </TabsContent>

        <TabsContent value="code" className="live-work-tab-panel live-work-code">
          <header className="live-work-tab-heading">
            <span><p className="pact-kicker">CÓDIGO EN VIVO</p><h2>Estado observado del código</h2></span>
          </header>
          <div className="live-work-code-status"><CodeActivityStatus activity={codeActivity} /></div>
        </TabsContent>

        <TabsContent value="handoffs" className="live-work-tab-panel live-work-handoffs">
          <header className="live-work-tab-heading">
            <span><p className="pact-kicker">CONTINUIDAD</p><h2>Handoffs estructurados</h2></span>
            <small>{handoffs.length} {handoffs.length === 1 ? "handoff" : "handoffs"}</small>
          </header>
          <HandoffList handoffs={handoffs} />
        </TabsContent>
      </div>
    </Tabs>
  );
}
