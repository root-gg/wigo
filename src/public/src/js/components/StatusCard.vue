<template>
  <div class="card status-card my-4">
    <div
      :class="['card-header', 'text-white', bgClass, { clickable: clickable }]"
      @click="handleClick"
    >
      <div class="d-flex align-items-center justify-content-between w-100">
        <span :class="{ 'fw-bold': clickable }">
          <slot name="title"></slot>
        </span>
        <div class="d-flex align-items-center gap-2">
          <slot name="badges"></slot>
        </div>
      </div>
    </div>
    <div class="card-body">
      <slot name="body"></slot>
    </div>
  </div>
</template>

<script setup>
import { computed } from "vue";
import { getBgLevelClass } from "../utils/status.js";

const props = defineProps({
  level: {
    type: String,
    required: true,
  },
  clickable: {
    type: Boolean,
    default: false,
  },
});

const emit = defineEmits(["click"]);

const bgClass = computed(() => getBgLevelClass(props.level));

function handleClick() {
  if (props.clickable) {
    emit("click");
  }
}
</script>

<style scoped>
.status-card .clickable {
  cursor: pointer;
}
</style>
