/*
 * Evolution Passkey Helper - ponte WebAuthn (MAIN world)
 * ------------------------------------------------------
 * Roda no contexto da propria pagina web.whatsapp.com ("world": "MAIN") e faz
 * apenas navigator.credentials.get(). Gerenciadores de senha como o 1Password
 * interceptam o WebAuthn da pagina, mas nao o do content script isolado: de
 * la o Chrome pedia "insira sua chave de seguranca" como se so existisse
 * chave fisica. (#173)
 *
 * Nao faz nenhuma chamada HTTP: a CSP do WhatsApp Web bloquearia a API do
 * Evolution GO a partir deste contexto. O content script conversa com a API e
 * troca o desafio e a assinatura com este script por eventos da janela.
 */
(function () {
  "use strict";

  var REQUEST_EVENT = "evo-wapk:webauthn-request";
  var RESULT_EVENT = "evo-wapk:webauthn-result";

  function b64uToBuf(value) {
    var b64 = String(value || "").replace(/-/g, "+").replace(/_/g, "/");
    while (b64.length % 4) b64 += "=";
    var bin = atob(b64);
    var bytes = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return bytes.buffer;
  }

  function bufToB64u(buf) {
    var bytes = new Uint8Array(buf);
    var bin = "";
    for (var i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
    return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
  }

  function buildPublicKeyOptions(pk) {
    var options = {
      challenge: b64uToBuf(pk.challenge),
      timeout: pk.timeout || 60000,
      rpId: pk.rpId || "whatsapp.com",
      userVerification: pk.userVerification || "required",
    };
    // allowCredentials vazio ou transports forcados restringem o seletor do
    // navegador e escondem o gerenciador de senhas; sem eles o navegador
    // oferece todas as chaves de acesso disponiveis para o dominio.
    if (Array.isArray(pk.allowCredentials) && pk.allowCredentials.length > 0) {
      options.allowCredentials = pk.allowCredentials.map(function (c) {
        return { type: c.type || "public-key", id: b64uToBuf(c.id) };
      });
    }
    return options;
  }

  function toWebAuthnResponse(cred) {
    var r = cred.response;
    var body = {
      id: cred.id,
      rawId: bufToB64u(cred.rawId),
      type: cred.type,
      response: {
        clientDataJSON: bufToB64u(r.clientDataJSON),
        authenticatorData: bufToB64u(r.authenticatorData),
        signature: bufToB64u(r.signature),
      },
    };
    if (r.userHandle && r.userHandle.byteLength) {
      body.response.userHandle = bufToB64u(r.userHandle);
    }
    return body;
  }

  function reply(requestId, payload) {
    payload.requestId = requestId;
    window.dispatchEvent(new CustomEvent(RESULT_EVENT, { detail: JSON.stringify(payload) }));
  }

  window.addEventListener(REQUEST_EVENT, function (event) {
    var detail;
    try {
      detail = JSON.parse(event.detail || "{}");
    } catch (e) {
      return;
    }
    if (!detail.requestId || !detail.publicKey) return;

    navigator.credentials
      .get({ publicKey: buildPublicKeyOptions(detail.publicKey) })
      .then(function (cred) {
        if (!cred) throw new Error("Autenticacao cancelada.");
        reply(detail.requestId, { result: toWebAuthnResponse(cred) });
      })
      .catch(function (e) {
        reply(detail.requestId, { error: (e && e.message) || String(e) });
      });
  });
})();
