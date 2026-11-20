// SCIM 2.0 stubs
import { Hono } from "hono";
const scim = new Hono();

scim.get("/Users", (c) => c.json({ totalResults: 0, Resources: [] }));
scim.post("/Users", (c) => c.json({ id: crypto.randomUUID() }, 201));
scim.patch("/Users/:id", (c) => c.json({ id: c.req.param("id") }));
scim.get("/Groups", (c) => c.json({ totalResults: 0, Resources: [] }));

export default scim;
