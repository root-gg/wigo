import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";

import "../scss/styles.scss";
import * as bootstrap from "bootstrap";

// Font Awesome
import "@fortawesome/fontawesome-free/css/all.min.css";

import { initTheme } from "./composables/useTheme.js";

initTheme();

const app = createApp(App);
app.use(router);
app.mount("#app");
