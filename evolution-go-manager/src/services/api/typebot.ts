/**
 * Typebot API Service
 *
 * Fala com as rotas /typebot deste fork do Evolution GO. Elas são autenticadas
 * pelo TOKEN DA INSTÂNCIA, não pela apikey global — por isso todas as chamadas
 * passam o header `apikey` explicitamente: o interceptor em client.ts só injeta
 * a chave global quando a requisição não trouxe uma.
 *
 * O contrato difere do Evolution API: lá a configuração é genérica para sete
 * integrações e inclui debounceTime, keepOpen, fallback e triggerType. Aqui só
 * existe o que o backend implementa.
 */

import apiClient from './client';

export type TypebotSessionStatus = 'opened' | 'paused' | 'closed';

export interface Typebot {
  id: string;
  instanceId: string;
  enabled: boolean;
  description: string;
  url: string;
  typebot: string;
  /** Minutos sem interação até a sessão encerrar. 0 desliga a expiração. */
  expire: number;
  keywordFinish: string;
  unknownMessage: string;
  /** Pausa em milissegundos antes de cada mensagem enviada. */
  delayMessage: number;
  listeningFromMe: boolean;
  stopBotFromMe: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface TypebotSession {
  id: string;
  instanceId: string;
  typebotId: string;
  remoteJid: string;
  pushName: string;
  sessionId: string;
  status: TypebotSessionStatus;
  awaitUser: boolean;
  createdAt: string;
  updatedAt: string;
}

/**
 * Campos aceitos na criação e na atualização.
 *
 * Os booleanos e numéricos são opcionais para permitir atualização parcial: o
 * backend só grava o que vier no corpo, então omitir um campo o preserva em vez
 * de zerá-lo.
 */
export interface TypebotPayload {
  enabled?: boolean;
  description?: string;
  url?: string;
  typebot?: string;
  expire?: number;
  keywordFinish?: string;
  unknownMessage?: string;
  delayMessage?: number;
  listeningFromMe?: boolean;
  stopBotFromMe?: boolean;
}

const instanceAuth = (instanceToken: string) => ({
  headers: { apikey: instanceToken },
});

export async function listTypebots(instanceToken: string): Promise<Typebot[]> {
  const response = await apiClient.get<Typebot[]>('/typebot', instanceAuth(instanceToken));
  return response.data ?? [];
}

export async function createTypebot(instanceToken: string, payload: TypebotPayload): Promise<Typebot> {
  const response = await apiClient.post<Typebot>('/typebot', payload, instanceAuth(instanceToken));
  return response.data;
}

export async function updateTypebot(
  instanceToken: string,
  id: string,
  payload: TypebotPayload,
): Promise<Typebot> {
  const response = await apiClient.put<Typebot>(`/typebot/${id}`, payload, instanceAuth(instanceToken));
  return response.data;
}

export async function deleteTypebot(instanceToken: string, id: string): Promise<void> {
  await apiClient.delete(`/typebot/${id}`, instanceAuth(instanceToken));
}

export async function listTypebotSessions(instanceToken: string): Promise<TypebotSession[]> {
  const response = await apiClient.get<TypebotSession[]>('/typebot/sessions', instanceAuth(instanceToken));
  return response.data ?? [];
}

/** Pausar, encerrar ou reabrir uma conversa em andamento. */
export async function setTypebotSessionStatus(
  instanceToken: string,
  sessionId: string,
  status: TypebotSessionStatus,
): Promise<void> {
  await apiClient.put(`/typebot/sessions/${sessionId}/status`, { status }, instanceAuth(instanceToken));
}

export async function deleteTypebotSession(instanceToken: string, sessionId: string): Promise<void> {
  await apiClient.delete(`/typebot/sessions/${sessionId}`, instanceAuth(instanceToken));
}
