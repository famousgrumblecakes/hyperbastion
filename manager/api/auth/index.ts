import type { Middleware } from "../../ext_modules/route-router";

//Placeholder middleware
const enforce_login: Middleware = async (ctx, next) => {
    let auth = ctx.req.cookies.get("auth");
    if (!auth) {
        return Response.redirect("/login");
    }

    ctx.meta.auth = {};

    return next();
};

export default enforce_login;
