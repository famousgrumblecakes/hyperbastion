import type { HTTPContext } from "../../ext_modules/route-router";
import {
    Route,
    Router,
    genericRejectHandler,
} from "../../ext_modules/route-router";
import { randomUUIDv7 } from "bun";
import { mkdir, readdir } from "fs/promises";
import { join } from "path";

// Allowed base images for container creation.
// TODO: replace this static list with a database query once the schema is ready.
const ALLOWED_BASE_IMAGES = ["ubuntu-26.04", "rocky-10.1"] as const;
type BaseImage = (typeof ALLOWED_BASE_IMAGES)[number];

// Where container spec JSON files are persisted on disk.
// TODO: make this configurable via an environment variable or config file.
const DATA_DIR = "./data/containers";

interface NFSMount {
    // Remote NFS export path, e.g. "192.168.1.100:/exports/shared"
    share_path: string;
    // Absolute path inside the container where the share will be mounted, e.g. "/mnt/shared"
    mount_point: string;
}

export interface ContainerSpec {
    id: string;
    owner: string;
    name: string;
    base_image: BaseImage;
    nfs_mount?: NFSMount;
    created_at: string;
    // TODO: candidates for future spec fields (all map to systemd-nspawn flags or unit properties):
    // hostname?: string;           // --hostname
    // memory_limit?: string;       // MemoryMax, e.g. "4G"
    // cpu_quota?: number;          // CPUQuota as a percentage, e.g. 50
    // network_zone?: string;       // --network-zone for a shared virtual ethernet bridge
    // extra_bind_mounts?: { host_path: string; container_path: string }[];
    // private_network?: boolean;   // --private-network for full network namespace isolation
    // ephemeral?: boolean;         // --ephemeral, discards overlay on shutdown
}

// Ensure a per-user data directory exists and return its path.
async function userDataDir(username: string): Promise<string> {
    const dir = join(DATA_DIR, username);
    await mkdir(dir, { recursive: true });
    return dir;
}

async function handleCreate(ctx: HTTPContext<string>): Promise<Response> {
    const username: string = ctx.meta.auth.username;

    let body: any;
    try {
        body = await ctx.req.json();
    } catch {
        return Response.json({ error: "Invalid JSON body" }, { status: 400 });
    }

    const { name, base_image, nfs_mount } = body;

    if (!name || typeof name !== "string" || !name.trim()) {
        return Response.json({ error: "'name' is required" }, { status: 400 });
    }

    if (!ALLOWED_BASE_IMAGES.includes(base_image)) {
        return Response.json(
            {
                error: `'base_image' must be one of: ${ALLOWED_BASE_IMAGES.join(", ")}`,
                allowed: ALLOWED_BASE_IMAGES,
            },
            { status: 400 },
        );
    }

    if (nfs_mount !== undefined) {
        if (
            typeof nfs_mount !== "object" ||
            typeof nfs_mount.share_path !== "string" ||
            !nfs_mount.share_path.trim() ||
            typeof nfs_mount.mount_point !== "string" ||
            !nfs_mount.mount_point.trim()
        ) {
            return Response.json(
                {
                    error: "When provided, 'nfs_mount' must contain non-empty 'share_path' and 'mount_point' strings",
                },
                { status: 400 },
            );
        }
    }

    const spec: ContainerSpec = {
        id: randomUUIDv7(),
        owner: username,
        name: name.trim(),
        base_image,
        created_at: new Date().toISOString(),
        ...(nfs_mount
            ? {
                  nfs_mount: {
                      share_path: nfs_mount.share_path.trim(),
                      mount_point: nfs_mount.mount_point.trim(),
                  },
              }
            : {}),
    };

    const dir = await userDataDir(username);
    await Bun.write(
        join(dir, `${spec.id}.json`),
        JSON.stringify(spec, null, 2),
    );

    return Response.json(spec, { status: 201 });
}

async function handleList(ctx: HTTPContext<string>): Promise<Response> {
    const username: string = ctx.meta.auth.username;

    const dir = await userDataDir(username);
    const files = await readdir(dir);

    const containers: ContainerSpec[] = [];
    for (const file of files.filter((f) => f.endsWith(".json"))) {
        try {
            const raw = await Bun.file(join(dir, file)).text();
            containers.push(JSON.parse(raw));
        } catch {
            // TODO: log malformed or unreadable spec files rather than silently skipping them
        }
    }

    return Response.json(containers);
}

const containerRoute = new Route("/", {
    handlers: {
        GET: handleList,
        POST: handleCreate,
    },
});

// Include this in the root Router via router.include(containerRouter).
export const containerRouter = new Router("/container", {
    routes: [containerRoute],
    rejectHandler: genericRejectHandler,
});
