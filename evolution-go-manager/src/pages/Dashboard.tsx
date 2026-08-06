import { useEffect, useState } from 'react';
import useAuth from '@/hooks/useAuth';

/**
 * Dashboard.
 *
 * A página em si é servida pelo próprio Evolution GO em GET /dashboard — um
 * HTML estático que consome /instance/all, /server/ok, /server/stats e
 * /instance/logs/:id. Aqui ela é apenas embutida.
 *
 * O parâmetro embed=1 faz o próprio HTML esconder o cabeçalho dele, para não
 * duplicar título e logo dentro do manager.
 *
 * Vale registrar por que é um iframe e não um port do conteúdo: a página tem
 * gráficos, polling e estado próprios, e reescrevê-la em React duplicaria a
 * manutenção sem ganho visível. Ela também continua acessível direto em
 * /dashboard, fora do manager.
 */

// O dashboard guarda credenciais nas próprias chaves. Escrevê-las aqui evita um
// segundo login: a origem é a mesma, então o localStorage é compartilhado.
const DASH_KEY = 'egogo_dash_key';
const DASH_BASE = 'egogo_dash_base';

function Dashboard() {
  const { apiUrl, apiKey } = useAuth();
  const [ready, setReady] = useState(false);

  useEffect(() => {
    if (!apiKey) return;

    // Base vazia faz o dashboard usar caminhos relativos, que é o certo quando
    // ele é servido pela mesma origem — em produção pelo Evolution GO, em
    // desenvolvimento pelo proxy do Vite. Só quando o manager aponta para uma
    // API em outro host é que a URL absoluta é necessária.
    const sameOrigin = !apiUrl || apiUrl.replace(/\/+$/, '') === window.location.origin;

    localStorage.setItem(DASH_KEY, apiKey);
    localStorage.setItem(DASH_BASE, sameOrigin ? '' : apiUrl.replace(/\/+$/, ''));

    // O iframe só é montado depois da escrita: se carregar antes, o dashboard
    // lê o localStorage vazio e mostra a tela de login dele.
    setReady(true);
  }, [apiUrl, apiKey]);

  if (!ready) {
    return (
      <div className="p-6">
        <p className="text-muted-foreground">Carregando dashboard...</p>
      </div>
    );
  }

  return (
    <iframe
      src="/dashboard?embed=1"
      title="Dashboard"
      className="h-[calc(100vh-4rem)] w-full border-0"
    />
  );
}

export default Dashboard;
