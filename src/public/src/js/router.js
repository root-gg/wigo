import { createRouter, createWebHashHistory } from "vue-router";
import Hosts from "./views/Hosts.vue";
import Group from "./views/Group.vue";
import Host from "./views/Host.vue";
import Logs from "./views/Logs.vue";
import Quiet from "./views/Quiet.vue";
import Tokens from "./views/Tokens.vue";
import Authority from "./views/Authority.vue";
import Disabled from "./views/Disabled.vue";

const routes = [
  {
    path: "/",
    name: "hosts",
    component: Hosts,
  },
  {
    path: "/group",
    name: "group",
    component: Group,
  },
  {
    path: "/host",
    name: "host",
    component: Host,
  },
  {
    path: "/logs",
    name: "logs",
    component: Logs,
  },
  {
    path: "/authority",
    name: "authority",
    component: Authority,
  },
  {
    path: "/quiet",
    name: "quiet",
    component: Quiet,
  },
  {
    path: "/tokens",
    name: "tokens",
    component: Tokens,
  },
  {
    path: "/wall",
    name: "wall",
    component: () => import("./views/Wall.vue"),
  },
  {
    path: "/disabled",
    name: "disabled",
    component: Disabled,
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) {
      return savedPosition;
    } else if (to.hash) {
      return {
        el: to.hash,
        behavior: "smooth",
      };
    } else {
      return { top: 0 };
    }
  },
});

export default router;
