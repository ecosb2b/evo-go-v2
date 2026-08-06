import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@components': path.resolve(__dirname, './src/components'),
      '@pages': path.resolve(__dirname, './src/pages'),
      '@services': path.resolve(__dirname, './src/services'),
      '@hooks': path.resolve(__dirname, './src/hooks'),
      '@store': path.resolve(__dirname, './src/store'),
      '@types': path.resolve(__dirname, './src/types'),
      '@utils': path.resolve(__dirname, './src/utils'),
      '@styles': path.resolve(__dirname, './src/styles'),
    },
  },
  server: {
    host: true,
    port: 5174,
    strictPort: true,
    cors: true,
    // Em produção o manager é servido pelo próprio Evolution GO, então
    // window.location.origin já aponta para a API. Em dev ele roda no Vite, e
    // sem proxy as chamadas caem no dev server — que responde o index.html do
    // SPA para qualquer rota, fazendo o login falhar ao tentar ler JSON.
    //
    // O alvo é configurável por VITE_API_TARGET para apontar o dev a uma API
    // remota sem editar este arquivo.
    proxy: Object.fromEntries(
      [
        '/instance',
        '/typebot',
        '/license',
        // Página estática servida pelo Evolution GO e embutida na aba Dashboard.
        // Precisa vir pelo proxy para ficar na mesma origem do manager — é o que
        // permite compartilhar o localStorage e dispensar um segundo login.
        '/dashboard',
        '/message',
        '/chat',
        '/group',
        '/send',
        '/user',
        '/server',
        '/label',
        '/unlabel',
        '/polls',
        '/newsletter',
        '/call',
        '/community',
        '/passkey-ceremony',
        '/health',
        '/swagger',
      ].map((path) => [
        path,
        {
          target: process.env.VITE_API_TARGET || 'http://localhost:8080',
          changeOrigin: true,
        },
      ]),
    ),
  },
  optimizeDeps: {
    include: [
      'react',
      'react-dom',
      'react-router-dom',
      '@evoapi/design-system',
      'lucide-react',
      'zustand',
    ],
  },
});
