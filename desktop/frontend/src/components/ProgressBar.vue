<script setup lang="ts">
import { X } from 'lucide-vue-next';
defineProps<{
  active: boolean;
  op: string;
  percent: number;
  message: string;
  queueLength?: number;
}>();

defineEmits(['cancel']);
</script>

<template>
  <Transition name="slide-down">
    <div v-if="active" class="progress-container">
      <div class="progress-info">
        <span class="op-type">
          {{ op.toUpperCase() }} IN PROGRESS
          <span v-if="queueLength && queueLength > 0" class="queue-badge">(+ {{ queueLength }} in queue)</span>
        </span>
        <div class="right-info">
          <span class="percent">{{ Math.round(percent) }}%</span>
          <button class="cancel-btn" @click.stop="$emit('cancel', op)" title="Cancel Operation">
            <X :size="16" />
          </button>
        </div>
      </div>
      <div class="progress-bar-bg">
        <div class="progress-bar-fill" :style="{ width: percent + '%' }"></div>
      </div>
      <div class="progress-message">{{ message }}</div>
    </div>
  </Transition>
</template>

<style scoped>
.progress-container {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border);
  padding: 12px 20px;
  z-index: 9999;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
}

.progress-info {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}

.op-type {
  font-size: 11px;
  font-weight: 700;
  color: var(--primary);
  letter-spacing: 0.05em;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.queue-badge {
  background: rgba(11, 150, 132, 0.15);
  color: var(--primary);
  padding: 2px 6px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: normal;
}

.right-info {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.percent {
  font-size: 13px;
  font-weight: 600;
}

.cancel-btn {
  background: none;
  border: none;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border-radius: 4px;
  color: var(--text-dim);
  cursor: pointer;
  transition: all 0.2s;
}

.cancel-btn:hover {
  background: rgba(255, 60, 60, 0.1);
  color: #ff4d4d;
}

.progress-bar-bg {
  height: 6px;
  background: var(--bg-app);
  border-radius: 3px;
  overflow: hidden;
  margin-bottom: 6px;
}

.progress-bar-fill {
  height: 100%;
  background: var(--primary);
  transition: width 0.3s ease-out;
}

.progress-message {
  font-size: 12px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Transitions */
.slide-down-enter-active,
.slide-down-leave-active {
  transition: transform 0.3s ease, opacity 0.3s ease;
}

.slide-down-enter-from,
.slide-down-leave-to {
  transform: translateY(-100%);
  opacity: 0;
}
</style>
