console.log("Hello via Bun!");

import enforce_login from "./api/auth";
import { usersRouter } from "./api/users";
import { Route, Router } from "./ext_modules/route-router";

const route = new Route("/", {
    handlers: {
        GET: async (ctx) => {
            return new Response("ok");
        },
    },
});

const api = new Router("/", {
    routes: [route],
    routers: [usersRouter],
    middlewares: [],
});

Bun.serve({
    port: 3000,
    routes: api.compile(),
});
