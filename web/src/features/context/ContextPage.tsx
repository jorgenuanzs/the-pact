import { Page } from "@/components/layout/Page";
import { LoadingState } from "@/components/ui/States";
import { useWorkspaceContext } from "@/features/overview/queries";
import { useWorkspace } from "@/features/workspaces/WorkspaceContext";

import { KnowledgeBoard } from "./KnowledgeBoard";

export function ContextPage() {
  const { workspace } = useWorkspace();
  const query = useWorkspaceContext(workspace?.id);
  if (query.isPending) return <LoadingState label="Cargando contexto" />;
  return <Page kicker="MEMORIA COMPARTIDA" title="Contexto" description="Decisiones, requisitos, restricciones, preguntas, riesgos y fuentes durables. El contexto no se convierte en verdad solo por aparecer en una conversación."><KnowledgeBoard context={query.data} /></Page>;
}
