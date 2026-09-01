import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { Code, ConnectError, type Interceptor } from "@connectrpc/connect";

import { getApiKey, notifyUnauthorized, setApiKey } from "./client";

/*
 * The gRPC-Web transport every generated service client shares. Mirrors
 * client.ts's REST request() in both directions:
 *  - outgoing: the same X-Api-Key header, read from the same in-memory/
 *    sessionStorage-backed key client.ts already owns.
 *  - incoming: a Code.Unauthenticated error triggers the same
 *    clear-the-key-and-notify-listeners behavior a 401 does today, so
 *    AuthContext's onUnauthorized(clearSession) subscription needs no
 *    change at all as domains migrate off REST.
 */
const authInterceptor: Interceptor = (next) => async (req) => {
  const apiKey = getApiKey();
  if (apiKey) {
    req.header.set("X-Api-Key", apiKey);
  }
  try {
    return await next(req);
  } catch (cause) {
    if (cause instanceof ConnectError && cause.code === Code.Unauthenticated) {
      setApiKey(null);
      notifyUnauthorized();
    }
    throw cause;
  }
};

export const transport = createGrpcWebTransport({
  baseUrl: window.location.origin,
  interceptors: [authInterceptor],
});
