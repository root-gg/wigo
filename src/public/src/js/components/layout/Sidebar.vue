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
      <!-- Le titre va sur le bouton, pas sur l'icône : une infobulle posée sur
           un enfant ne s'affiche que si le pointeur touche l'icône elle-même,
           et pas le reste de la zone cliquable. -->
      <button
        type="button"
        class="btn btn-link text-white sidebar__toggle p-0 border-0"
        :aria-label="collapsed ? 'Expand the sidebar' : 'Collapse the sidebar'"
        :aria-expanded="!collapsed"
        :title="collapsed ? 'Expand the sidebar' : 'Collapse the sidebar'"
        @click="$emit('toggle')"
      >
        <i
          :class="['fas', collapsed ? 'fa-chevron-right' : 'fa-chevron-left']"
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

/*
 * Sur un téléphone, un rail de 56px n'est pas une barre latérale repliée :
 * c'est 14% de la largeur occupés par des pictogrammes tous identiques. En
 * dessous de md, elle sort donc de l'écran au lieu de rétrécir, et le contenu
 * prend toute la place.
 */
@media (max-width: 767.98px) {
  .sidebar {
    position: fixed;
    top: 0;
    left: 0;
    height: 100dvh;
    z-index: 1045;
    width: min(280px, 85vw);
    min-width: min(280px, 85vw);
    transition: transform 0.2s ease;
    box-shadow: 0 0 2rem rgba(0, 0, 0, 0.4);
  }

  /* Repliée veut dire fermée, pas étroite */
  .sidebar--reduced {
    width: min(280px, 85vw);
    min-width: min(280px, 85vw);
    transform: translateX(-100%);
    box-shadow: none;
  }

  /* ... et rien n'est caché dedans, puisqu'elle est ouverte ou absente */
  .sidebar--reduced .sidebar__header {
    justify-content: space-between;
    padding-left: 1rem;
    padding-right: 1rem;
  }
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

@media (max-width: 767.98px) {
  /* Le tiroir fermé est hors écran, pas réduit : son contenu reste entier */
  .sidebar--reduced .nav-link > *:not(i) {
    display: inline !important;
  }
}
</style>
