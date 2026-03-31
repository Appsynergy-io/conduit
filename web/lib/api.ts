import createClient from "openapi-fetch";

// API client for the Conduit server.
// In static export mode, the frontend is served from the same origin as the API,
// so we use a relative base URL.
const api = createClient({
  baseUrl: "/api/v1",
  headers: {
    "Content-Type": "application/json",
  },
});

// Attach JWT token to requests if available.
export function setAuthToken(token: string | null) {
  if (token) {
    api.use({
      onRequest({ request }) {
        request.headers.set("Authorization", `Bearer ${token}`);
        return request;
      },
    });
  }
}

export default api;
