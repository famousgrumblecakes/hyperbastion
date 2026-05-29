import { SQL } from "bun";
import type { HTTPContext } from "../../ext_modules/route-router";
import {
    Route,
    Router,
    genericRejectHandler,
} from "../../ext_modules/route-router";
import { db } from "../../db";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface UserRow {
    id: number;
    username: string;
    email: string;
    display_name: string | null;
    auth_provider: string;
    provider_user_id: string | null;
    password_hash: string | null;
    is_active: number;
    created_at: string;
    updated_at: string;
    last_login_at: string | null;
}

// password_hash is never included in API responses.
type UserResponse = Omit<UserRow, "password_hash">;

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

async function handleList(_ctx: HTTPContext<string>): Promise<Response> {
    const users = (await db`
        SELECT id, username, email, display_name, auth_provider,
               provider_user_id, is_active, created_at, updated_at, last_login_at
        FROM users
        ORDER BY created_at DESC
    `) as UserResponse[];

    return Response.json(users);
}

async function handleGetById(ctx: HTTPContext<string>): Promise<Response> {
    const id = Number(ctx.req.params.id);
    if (!Number.isInteger(id) || id < 1) {
        return Response.json({ error: "Invalid user ID" }, { status: 400 });
    }

    const rows = (await db`
        SELECT id, username, email, display_name, auth_provider,
               provider_user_id, is_active, created_at, updated_at, last_login_at
        FROM users
        WHERE id = ${id}
    `) as UserResponse[];

    const user = rows[0];
    if (!user)
        return Response.json({ error: "User not found" }, { status: 404 });

    return Response.json(user);
}

async function handleCreate(ctx: HTTPContext<string>): Promise<Response> {
    let body: any;
    try {
        body = await ctx.req.json();
    } catch {
        return Response.json({ error: "Invalid JSON body" }, { status: 400 });
    }

    const { username, email, password } = body;

    if (!username || typeof username !== "string" || !username.trim()) {
        return Response.json(
            { error: "'username' is required" },
            { status: 400 },
        );
    }
    if (!email || typeof email !== "string" || !email.trim()) {
        return Response.json({ error: "'email' is required" }, { status: 400 });
    }
    if (!password || typeof password !== "string" || password.length < 8) {
        return Response.json(
            { error: "'password' must be at least 8 characters" },
            { status: 400 },
        );
    }

    const display_name: string | null =
        typeof body.display_name === "string" && body.display_name.trim()
            ? body.display_name.trim()
            : null;

    const password_hash = await Bun.password.hash(password);

    let rows: UserResponse[];
    try {
        rows = (await db`
            INSERT INTO users (username, email, display_name, password_hash)
            VALUES (
                ${username.trim()},
                ${email.trim().toLowerCase()},
                ${display_name},
                ${password_hash}
            )
            RETURNING id, username, email, display_name, auth_provider,
                      provider_user_id, is_active, created_at, updated_at, last_login_at
        `) as UserResponse[];
    } catch (err: unknown) {
        if (
            err instanceof SQL.SQLiteError &&
            err.code === "SQLITE_CONSTRAINT"
        ) {
            return Response.json(
                { error: "Username or email already in use" },
                { status: 409 },
            );
        }
        throw err;
    }

    const user = rows[0];
    if (!user) throw new Error("INSERT returned no rows");

    return Response.json(user, { status: 201 });
}

async function handleDelete(ctx: HTTPContext<string>): Promise<Response> {
    const id = Number(ctx.req.params.id);
    if (!Number.isInteger(id) || id < 1) {
        return Response.json({ error: "Invalid user ID" }, { status: 400 });
    }

    // RETURNING lets us detect a no-op delete without a separate SELECT.
    const deleted = (await db`
        DELETE FROM users WHERE id = ${id} RETURNING id
    `) as { id: number }[];

    if (!deleted[0]) {
        return Response.json({ error: "User not found" }, { status: 404 });
    }

    return new Response(null, { status: 204 });
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

// Collection: /users
const collectionRoute = new Route("/", {
    handlers: {
        GET: handleList,
        POST: handleCreate,
    },
});

// Individual users: /users/:id
const userRoute = new Route<string>("/:id", {
    handlers: {
        GET: handleGetById,
        DELETE: handleDelete,
    },
});

// Include this in the root Router via router.include(usersRouter).
export const usersRouter = new Router("/users", {
    routes: [collectionRoute, userRoute],
    rejectHandler: genericRejectHandler,
});
