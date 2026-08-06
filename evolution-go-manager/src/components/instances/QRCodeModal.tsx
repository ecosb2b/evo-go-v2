/**
 * QRCode Modal Component
 * Displays QR code for WhatsApp connection
 */

import { useEffect, useRef, useState } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Button,
} from '@evoapi/design-system';
import { QrCode, RefreshCw, X, CheckCircle2 } from 'lucide-react';
import { toast } from 'sonner';
import type { Instance } from '@/types/instance';

/**
 * De quanto em quanto tempo o modal consulta a API.
 *
 * Mais curto que a rotação do QR de propósito: além de buscar o código, cada
 * consulta detecta se o pareamento já aconteceu. Esperar a rotação inteira
 * deixaria a tela de sucesso demorar para aparecer depois do escaneamento.
 */
const POLL_SECONDS = 5;

/**
 * Vida útil aproximada de um QR do WhatsApp, usada só para exibição.
 *
 * O contador na tela não conta o polling — ele conta a vida do código, e é
 * reiniciado quando um QR diferente chega. Sem isso o número mostrado não teria
 * relação com o que o usuário vê: o polling é mais rápido que a rotação, então
 * a maioria das consultas devolve o mesmo código e o contador zeraria sem nada
 * mudar na tela.
 */
const QR_LIFETIME_SECONDS = 20;

interface QRCodeModalProps {
  instance: Instance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onRefresh?: () => Promise<void>;
}

export default function QRCodeModal({
  instance,
  open,
  onOpenChange,
  onRefresh,
}: QRCodeModalProps) {
  const [isRefreshing, setIsRefreshing] = useState(false);

  // O WhatsApp rotaciona o QR a cada ~20s, então o modal busca um novo
  // periodicamente e, de quebra, detecta quando a conexão foi estabelecida.
  //
  // onRefresh fica numa ref e FORA das dependências de propósito: ele é
  // recriado a cada render (depende da instância que ele próprio atualiza), e
  // mantê-lo na lista fazia o efeito rodar de novo a cada atualização —
  // cancelando o timer e recomeçando a contagem antes dos 10s completarem. O
  // refresh automático simplesmente nunca disparava.
  const onRefreshRef = useRef(onRefresh);
  useEffect(() => {
    onRefreshRef.current = onRefresh;
  }, [onRefresh]);

  const isConnected = instance?.connected ?? false;

  const currentQr = instance?.qrcode?.base64;

  const countdownRef = useRef(QR_LIFETIME_SECONDS);
  const [secondsLeft, setSecondsLeft] = useState(QR_LIFETIME_SECONDS);

  // O contador acompanha o CÓDIGO, não a consulta: sempre que um QR diferente
  // chega, a contagem recomeça. É isso que faz o número na tela corresponder ao
  // que o usuário enxerga.
  useEffect(() => {
    if (!currentQr) return;
    countdownRef.current = QR_LIFETIME_SECONDS;
    setSecondsLeft(QR_LIFETIME_SECONDS);
  }, [currentQr]);

  useEffect(() => {
    if (!open || isConnected) {
      countdownRef.current = QR_LIFETIME_SECONDS;
      setSecondsLeft(QR_LIFETIME_SECONDS);
      return;
    }

    let secondsSincePoll = 0;

    const tick = setInterval(() => {
      // Chega a zero e para: o QR pode durar um pouco mais que o previsto, e
      // números negativos na tela não ajudariam ninguém. A próxima rotação
      // reinicia a contagem pelo efeito acima.
      if (countdownRef.current > 0) {
        countdownRef.current -= 1;
        setSecondsLeft(countdownRef.current);
      }

      secondsSincePoll += 1;
      if (secondsSincePoll >= POLL_SECONDS) {
        secondsSincePoll = 0;
        // Fora de qualquer setState: atualizadores podem rodar duas vezes em
        // StrictMode, o que dobraria as requisições.
        onRefreshRef.current?.().catch((err) => {
          console.error('Auto-refresh failed:', err);
        });
      }
    }, 1000);

    return () => clearInterval(tick);
    // Só o que deve reiniciar o ciclo: abrir/fechar o modal e conectar.
  }, [open, isConnected]);

  const handleRefresh = async () => {
    if (!onRefresh) return;

    setIsRefreshing(true);
    try {
      await onRefresh();
      toast.success('QR Code atualizado!');
    } catch (error) {
      console.error('Erro ao atualizar QR Code:', error);
      toast.error('Erro ao atualizar QR Code');
    } finally {
      setIsRefreshing(false);
    }
  };

  if (!instance) return null;

  // If already connected, show success message
  if (instance.connected) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-green-500">
              <CheckCircle2 className="h-5 w-5" />
              Conectado com Sucesso!
            </DialogTitle>
            <DialogDescription className="text-sidebar-foreground/70">
              A instância {instance.instanceName} foi conectada ao WhatsApp.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col items-center gap-4 py-6">
            <div className="rounded-full bg-green-500/10 p-4">
              <CheckCircle2 className="h-12 w-12 text-green-500" />
            </div>
            {instance.profileName && (
              <div className="text-center">
                <p className="text-sm text-sidebar-foreground/60">
                  Conectado como
                </p>
                <p className="text-lg font-semibold text-sidebar-foreground">
                  {instance.profileName}
                </p>
              </div>
            )}
          </div>

          <div className="flex justify-end">
            <Button
              onClick={() => onOpenChange(false)}
              className="w-full sm:w-auto"
            >
              Fechar
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    );
  }

  // Show QR Code
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <QrCode className="h-5 w-5 text-primary" />
            Conectar WhatsApp
          </DialogTitle>
          <DialogDescription className="text-sidebar-foreground/70">
            Escaneie o QR Code abaixo com seu WhatsApp para conectar a instância{' '}
            <strong>{instance.instanceName}</strong>
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {/* QR Code Display */}
          <div className="flex flex-col items-center gap-4">
            {instance.qrcode?.base64 ? (
              <div className="flex flex-col items-center gap-2">
                <div className="rounded-lg border-2 border-sidebar-border bg-white p-4">
                  <img
                    src={instance.qrcode.base64}
                    alt="QR Code"
                    className="h-64 w-64"
                  />
                </div>

                {/* Deixa visível que o código se renova sozinho — sem isso o
                    usuário não sabe se deve esperar ou clicar em atualizar. */}
                <p className="text-xs text-muted-foreground">
                  {isRefreshing
                    ? 'Atualizando...'
                    : secondsLeft > 0
                      ? `Novo código em ${secondsLeft}s`
                      : 'Aguardando novo código...'}
                </p>
              </div>
            ) : (
              <div className="flex h-64 w-64 items-center justify-center rounded-lg border-2 border-dashed border-sidebar-border bg-sidebar">
                <div className="text-center">
                  <QrCode className="mx-auto h-12 w-12 text-sidebar-foreground/40" />
                  <p className="mt-2 text-sm text-sidebar-foreground/60">
                    Aguardando QR Code...
                  </p>
                </div>
              </div>
            )}

            {/* Pairing Code (if available) */}
            {instance.qrcode?.pairingCode && (
              <div className="w-full rounded-lg bg-sidebar-accent p-3 text-center">
                <p className="text-xs text-sidebar-foreground/60">
                  Código de Pareamento
                </p>
                <p className="mt-1 font-mono text-lg font-semibold text-sidebar-foreground">
                  {instance.qrcode.pairingCode}
                </p>
              </div>
            )}
          </div>

          {/* Instructions */}
          <div className="rounded-lg bg-sidebar-accent p-4">
            <p className="text-sm font-medium text-sidebar-foreground">
              Como conectar:
            </p>
            <ol className="mt-2 space-y-1 text-sm text-sidebar-foreground/70">
              <li>1. Abra o WhatsApp no seu celular</li>
              <li>2. Toque em Menu ou Configurações</li>
              <li>3. Toque em Dispositivos conectados</li>
              <li>4. Toque em Conectar um dispositivo</li>
              <li>5. Aponte seu celular para esta tela para capturar o código</li>
            </ol>
          </div>

          {/* Actions */}
          <div className="flex gap-2">
            <Button
              variant="outline"
              onClick={handleRefresh}
              disabled={isRefreshing}
              className="flex-1 bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
            >
              {isRefreshing ? (
                <>
                  <RefreshCw className="mr-2 h-4 w-4 animate-spin" />
                  Atualizando...
                </>
              ) : (
                <>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  Atualizar QR Code
                </>
              )}
            </Button>
            <Button
              variant="outline"
              onClick={() => onOpenChange(false)}
              className="bg-sidebar border-sidebar-border text-sidebar-foreground hover:bg-sidebar-accent"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
