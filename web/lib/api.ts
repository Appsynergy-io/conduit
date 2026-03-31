import createClient from "openapi-fetch"

const api = createClient({
  baseUrl: "/api/v1",
  headers: {
    "Content-Type": "application/json",
  },
})

export default api
