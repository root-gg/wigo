<template>
  <div
    :class="[
      'sidebar',
      'd-flex',
      'flex-column',
      'flex-shrink-0',
      'align-self-stretch',
      'justify-content-start',
      'text-white',
      { 'sidebar--reduced': collapsed },
    ]"
    data-bs-theme="dark"
  >
    <div class="sidebar__header d-flex align-items-center px-3">
      <a
        v-show="!collapsed"
        href="/"
        class="wigo-logo sidebar__logo text-white text-decoration-none fs-4 me-auto"
      >
        W I G O
      </a>
      <button
        type="button"
        class="btn btn-link text-white sidebar__toggle p-0 border-0"
        aria-label="Réduire / agrandir la barre latérale"
        @click="$emit('toggle')"
      >
        <i
          :class="['fas', collapsed ? 'fa-chevron-right' : 'fa-chevron-left']"
          :title="collapsed ? 'Agrandir la sidebar' : 'Réduire la sidebar'"
        ></i>
      </button>
    </div>
    <hr class="border-white align-self-stretch my-0" />
    <ul class="nav flex-column align-self-stretch py-2 sidebar__nav">
      <slot></slot>
    </ul>
  </div>
</template>

<script setup>
defineProps({
  collapsed: {
    type: Boolean,
    default: false,
  },
});

defineEmits(["toggle"]);
</script>

<style scoped>
.sidebar {
  width: 280px;
  min-width: 280px;
  background-color: var(--wigo-sidebar-bg);
  transition:
    width 0.2s ease,
    min-width 0.2s ease;
  overflow-x: hidden;
  overflow-y: auto;
  box-sizing: border-box;
}

.nav {
  --bs-nav-link-color: rgb(143, 188, 255);
  --bs-nav-link-hover-color: rgb(200, 228, 255);
}

.sidebar--reduced {
  width: 56px;
  min-width: 56px;
}

.sidebar__header {
  min-height: var(--navbar-block-height);
  box-sizing: border-box;
}

.sidebar--reduced .sidebar__header {
  justify-content: center;
  padding-left: 0.5rem;
  padding-right: 0.5rem;
}

.sidebar--reduced .sidebar__header .sidebar__toggle {
  margin: 0;
}

.sidebar__toggle:hover {
  opacity: 0.7;
}

.sidebar__nav {
  padding-left: 0;
  padding-right: 0;
}
</style>

<style>
.sidebar li.sidebar-section-title,
.sidebar li.sidebar-section-title a {
  color: #fff;
}

.sidebar .nav-link > *:not(i) {
  font-size: 0.875rem;
}

.sidebar--reduced .nav-item {
  margin-top: 0;
}

.sidebar--reduced .nav-link > *:not(i) {
  display: none !important;
}
</style>
