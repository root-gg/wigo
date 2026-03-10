<template>
  <nav
    :class="[
      'navbar',
      'navbar-expand',
      'text-bg-light',
      'navbar-block',
      { 'sidebar-collapsed': sidebarCollapsed },
    ]"
    data-bs-theme="light"
  >
    <div class="container-fluid d-flex align-items-center">
      <a
        v-if="sidebarCollapsed"
        href="/"
        class="wigo-logo navbar-brand text-dark text-decoration-none ms-3 me-4"
      >
        W I G O
      </a>
      <div>
        <button
          v-for="level in ['OK', 'INFO', 'WARNING', 'CRITICAL', 'ERROR']"
          :key="level"
          type="button"
          :class="['btn', 'mx-1', getBtnClass(level)]"
        >
          <span class="d-none d-lg-inline">{{ level }}: </span>
          {{ counts[level] || 0 }}
        </button>
      </div>

      <ul class="navbar-nav ms-auto">
        <li class="nav-item dropdown">
          <a
            class="nav-link dropdown-toggle"
            href="#"
            role="button"
            data-bs-toggle="dropdown"
            aria-expanded="false"
            title="Refresh settings"
          >
            <i class="fas fa-sync"></i>
          </a>
          <ul class="dropdown-menu dropdown-menu-end">
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 5 }]"
                href="#"
                @click.prevent="setInterval(5)"
                title="Refresh every 5 seconds"
              >
                <i class="fas fa-gear fa-fw"></i> 5 sec
              </a>
            </li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 15 }]"
                href="#"
                @click.prevent="setInterval(15)"
                title="Refresh every 15 seconds"
              >
                <i class="fas fa-gear fa-fw"></i> 15 sec
              </a>
            </li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 30 }]"
                href="#"
                @click.prevent="setInterval(30)"
                title="Refresh every 30 seconds"
              >
                <i class="fas fa-gear fa-fw"></i> 30 sec
              </a>
            </li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 60 }]"
                href="#"
                @click.prevent="setInterval(60)"
                title="Refresh every 60 seconds"
              >
                <i class="fas fa-gear fa-fw"></i> 60 sec
              </a>
            </li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 300 }]"
                href="#"
                @click.prevent="setInterval(300)"
                title="Refresh every 300 seconds"
              >
                <i class="fas fa-gear fa-fw"></i> 300 sec
              </a>
            </li>
            <li><hr class="dropdown-divider" /></li>
            <li>
              <a
                :class="['dropdown-item', { active: currentInterval === 0 }]"
                href="#"
                @click.prevent="setInterval(0)"
                title="Disable auto-refresh"
              >
                <i class="fas fa-stop fa-fw"></i> Disable
              </a>
            </li>
          </ul>
        </li>
        <li class="nav-item">
          <router-link class="nav-link" to="/logs" title="View logs">
            <i class="fas fa-list fa-fw"></i>
          </router-link>
        </li>
        <li class="nav-item">
          <router-link
            class="nav-link"
            to="/authority"
            title="Authority settings"
          >
            <i class="fas fa-lock fa-fw"></i>
          </router-link>
        </li>
      </ul>
    </div>
  </nav>
</template>

<script setup>
import { onMounted, onUnmounted, ref } from "vue";
import { useRouter } from "vue-router";
import { getBtnLevelClass } from "../../utils/status.js";

const router = useRouter();

const props = defineProps({
  counts: {
    type: Object,
    default: () => ({
      OK: 0,
      INFO: 0,
      WARNING: 0,
      CRITICAL: 0,
      ERROR: 0,
    }),
  },
  sidebarCollapsed: {
    type: Boolean,
    default: false,
  },
  currentInterval: {
    type: Number,
    default: 0,
  },
});

const emit = defineEmits(["refresh-settings"]);

const windowWidth = ref(window.innerWidth);

function handleResize() {
  windowWidth.value = window.innerWidth;
}

onMounted(() => {
  window.addEventListener("resize", handleResize);
});

onUnmounted(() => {
  window.removeEventListener("resize", handleResize);
});

function setInterval(seconds) {
  emit("refresh-settings", seconds);
}

function getBtnClass(level) {
  return getBtnLevelClass(level);
}
</script>
