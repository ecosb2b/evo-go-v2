import { useCallback, useEffect, useState } from "react";
import { Bot, Loader2, Pause, Play, RefreshCw, Save, Trash2, X } from "lucide-react";
import { Button } from "@evoapi/design-system";
import { toast } from "sonner";
import * as typebotApi from "@/services/api/typebot";
import type { Typebot, TypebotSession } from "@/services/api/typebot";

/**
 * Configuração do Typebot para uma instância.
 *
 * As rotas /typebot são autenticadas pelo token DA INSTÂNCIA, e não pela apikey
 * global — por isso o componente recebe `instanceToken` e o repassa a cada
 * chamada.
 *
 * O backend deste fork implementa um subconjunto do que o Evolution API oferece:
 * não há debounceTime, keepOpen, fallback nem triggerType/regex. O formulário
 * mostra apenas o que existe, para não sugerir configurações que seriam
 * ignoradas.
 */

interface Props {
  instanceToken: string;
}

const emptyForm = {
  description: "",
  url: "",
  typebot: "",
  expire: 60,
  keywordFinish: "#sair",
  unknownMessage: "Desculpe, não entendi. Pode tentar novamente?",
  delayMessage: 0,
  listeningFromMe: false,
  stopBotFromMe: true,
  enabled: true,
};

const inputClass =
  "w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring";

export default function TypebotSettings({ instanceToken }: Props) {
  const [bot, setBot] = useState<Typebot | null>(null);
  const [form, setForm] = useState(emptyForm);
  const [sessions, setSessions] = useState<TypebotSession[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);

  const load = useCallback(async () => {
    if (!instanceToken) return;
    setIsLoading(true);
    try {
      const [bots, loadedSessions] = await Promise.all([
        typebotApi.listTypebots(instanceToken),
        typebotApi.listTypebotSessions(instanceToken),
      ]);

      // A tabela aceita vários bots por instância, mas o backend atende com o
      // primeiro habilitado — a tela reflete essa regra em vez de sugerir uma
      // lista que não seria usada.
      const current = bots[0] ?? null;
      setBot(current);
      setSessions(loadedSessions);

      if (current) {
        setForm({
          description: current.description,
          url: current.url,
          typebot: current.typebot,
          expire: current.expire,
          keywordFinish: current.keywordFinish,
          unknownMessage: current.unknownMessage,
          delayMessage: current.delayMessage,
          listeningFromMe: current.listeningFromMe,
          stopBotFromMe: current.stopBotFromMe,
          enabled: current.enabled,
        });
      }
    } catch (error) {
      toast.error((error as { message?: string })?.message ?? "Erro ao carregar o Typebot");
    } finally {
      setIsLoading(false);
    }
  }, [instanceToken]);

  useEffect(() => {
    load();
  }, [load]);

  const handleSave = async () => {
    if (!form.url.trim() || !form.typebot.trim()) {
      toast.error("URL e nome do fluxo são obrigatórios");
      return;
    }

    setIsSaving(true);
    try {
      const saved = bot
        ? await typebotApi.updateTypebot(instanceToken, bot.id, form)
        : await typebotApi.createTypebot(instanceToken, form);
      setBot(saved);
      toast.success("Typebot salvo");
    } catch (error) {
      toast.error((error as { message?: string })?.message ?? "Erro ao salvar");
    } finally {
      setIsSaving(false);
    }
  };

  const handleDelete = async () => {
    if (!bot) return;
    // Apagar o bot remove as sessões junto; sem ele elas não teriam para onde
    // continuar.
    if (!confirm("Remover a configuração do Typebot e todas as sessões desta instância?")) return;

    try {
      await typebotApi.deleteTypebot(instanceToken, bot.id);
      setBot(null);
      setSessions([]);
      setForm(emptyForm);
      toast.success("Typebot removido");
    } catch (error) {
      toast.error((error as { message?: string })?.message ?? "Erro ao remover");
    }
  };

  const handleSessionStatus = async (session: TypebotSession, status: typebotApi.TypebotSessionStatus) => {
    try {
      await typebotApi.setTypebotSessionStatus(instanceToken, session.id, status);
      setSessions((all) => all.map((s) => (s.id === session.id ? { ...s, status } : s)));
    } catch (error) {
      toast.error((error as { message?: string })?.message ?? "Erro ao atualizar a sessão");
    }
  };

  const handleSessionDelete = async (session: TypebotSession) => {
    try {
      await typebotApi.deleteTypebotSession(instanceToken, session.id);
      setSessions((all) => all.filter((s) => s.id !== session.id));
    } catch (error) {
      toast.error((error as { message?: string })?.message ?? "Erro ao remover a sessão");
    }
  };

  if (isLoading) {
    return (
      <div className="rounded-lg border border-sidebar-border bg-card p-6">
        <div className="flex items-center gap-2 text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          <span className="text-sm">Carregando Typebot...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-sidebar-border bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Bot className="h-5 w-5 text-primary" />
            <h2 className="text-lg font-semibold text-foreground">Typebot</h2>
          </div>

          <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
            <input
              type="checkbox"
              className="h-4 w-4 rounded border-input"
              checked={form.enabled}
              onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
            />
            Ativo
          </label>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="md:col-span-2">
            <label className="mb-1 block text-sm text-muted-foreground">Descrição</label>
            <input
              className={inputClass}
              value={form.description}
              onChange={(e) => setForm({ ...form, description: e.target.value })}
              placeholder="Atendimento principal"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-muted-foreground">URL do Typebot *</label>
            <input
              className={inputClass}
              value={form.url}
              onChange={(e) => setForm({ ...form, url: e.target.value })}
              placeholder="https://vw.exemplo.net"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-muted-foreground">Nome público do fluxo *</label>
            <input
              className={inputClass}
              value={form.typebot}
              onChange={(e) => setForm({ ...form, typebot: e.target.value })}
              placeholder="meu-bot-abc123"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-muted-foreground">
              Expiração (minutos) — 0 desativa
            </label>
            <input
              type="number"
              min={0}
              className={inputClass}
              value={form.expire}
              onChange={(e) => setForm({ ...form, expire: Number(e.target.value) })}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-muted-foreground">Palavra de encerramento</label>
            <input
              className={inputClass}
              value={form.keywordFinish}
              onChange={(e) => setForm({ ...form, keywordFinish: e.target.value })}
              placeholder="#sair"
            />
          </div>

          <div className="md:col-span-2">
            <label className="mb-1 block text-sm text-muted-foreground">
              Mensagem quando o fluxo não responde nada
            </label>
            <input
              className={inputClass}
              value={form.unknownMessage}
              onChange={(e) => setForm({ ...form, unknownMessage: e.target.value })}
            />
          </div>

          <div>
            <label className="mb-1 block text-sm text-muted-foreground">
              Atraso entre mensagens (ms)
            </label>
            <input
              type="number"
              min={0}
              className={inputClass}
              value={form.delayMessage}
              onChange={(e) => setForm({ ...form, delayMessage: Number(e.target.value) })}
            />
          </div>

          <div className="flex flex-col justify-end gap-2">
            <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-input"
                checked={form.stopBotFromMe}
                onChange={(e) => setForm({ ...form, stopBotFromMe: e.target.checked })}
              />
              Encerrar quando eu escrever na conversa
            </label>

            <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border-input"
                checked={form.listeningFromMe}
                onChange={(e) => setForm({ ...form, listeningFromMe: e.target.checked })}
              />
              Responder também às minhas mensagens
            </label>
          </div>
        </div>

        <div className="mt-6 flex items-center gap-2">
          <Button onClick={handleSave} disabled={isSaving}>
            {isSaving ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Save className="mr-2 h-4 w-4" />}
            Salvar
          </Button>

          {bot && (
            <Button variant="destructive" onClick={handleDelete}>
              <Trash2 className="mr-2 h-4 w-4" />
              Remover
            </Button>
          )}
        </div>
      </div>

      <div className="rounded-lg border border-sidebar-border bg-card p-6">
        <div className="mb-4 flex items-center justify-between">
          <h3 className="text-base font-semibold text-foreground">
            Conversas em andamento ({sessions.length})
          </h3>
          <Button variant="ghost" size="sm" onClick={load}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Atualizar
          </Button>
        </div>

        {sessions.length === 0 ? (
          <p className="text-sm text-muted-foreground">Nenhuma conversa ativa.</p>
        ) : (
          <div className="max-h-96 space-y-2 overflow-y-auto">
            {sessions.map((session) => (
              <div
                key={session.id}
                className="flex items-center justify-between gap-4 rounded-md border border-input p-3"
              >
                <div className="min-w-0">
                  <p className="truncate text-sm text-foreground">
                    {session.pushName || session.remoteJid}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {session.remoteJid} · {session.status}
                  </p>
                </div>

                <div className="flex shrink-0 items-center gap-1">
                  {session.status === "opened" ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      title="Pausar"
                      onClick={() => handleSessionStatus(session, "paused")}
                    >
                      <Pause className="h-4 w-4" />
                    </Button>
                  ) : (
                    <Button
                      variant="ghost"
                      size="sm"
                      title="Reabrir"
                      onClick={() => handleSessionStatus(session, "opened")}
                    >
                      <Play className="h-4 w-4" />
                    </Button>
                  )}

                  <Button
                    variant="ghost"
                    size="sm"
                    title="Encerrar"
                    onClick={() => handleSessionStatus(session, "closed")}
                  >
                    <X className="h-4 w-4" />
                  </Button>

                  <Button
                    variant="ghost"
                    size="sm"
                    title="Remover"
                    onClick={() => handleSessionDelete(session)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
