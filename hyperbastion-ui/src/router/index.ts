import { createRouter, createWebHistory } from "vue-router";

const router = createRouter({
    history: createWebHistory(import.meta.env.BASE_URL),
    routes: [
        {
            path: "/",
            component: () => import("../views/HomeView.vue"),
            name: "home",
        },
        {
            path: "/login",
            component: () => import("../views/LoginView.vue"),
            name: "login",
        },
    ],
});

export default router;
