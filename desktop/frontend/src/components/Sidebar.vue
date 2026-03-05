<script setup lang="ts">
import { HardDrive, Plus, Unlock } from 'lucide-vue-next';
import { computed, onMounted, onUnmounted, ref } from 'vue';

const props = defineProps<{
  vaults: { name: string, path: string }[]
  selectedVaultName?: string
  unlockedVaults: string[]
  isDark: boolean
  isInVault?: boolean
  appVersion?: string
}>()

const isCurrentVaultUnlocked = computed(() => {
    return props.selectedVaultName && props.unlockedVaults.includes(props.selectedVaultName)
})

const emit = defineEmits<{
  (e: 'select-vault', vault: { name: string, path: string }): void
  (e: 'create-vault'): void
  (e: 'toggle-theme'): void
  (e: 'create-folder'): void
  (e: 'upload-file'): void
  (e: 'upload-folder'): void
  (e: 'lock-vault'): void
  (e: 'upload-cross-vault', payload: { targetVault: string, sourceVault: string, ids: string[] }): void
}>()

const dragOverVault = ref<string | null>(null)

const onDragOver = (vault: { name: string }) => {
  if (props.unlockedVaults.includes(vault.name)) {
    dragOverVault.value = vault.name
  }
}

const onDragLeave = (vault: { name: string }) => {
  if (dragOverVault.value === vault.name) {
    dragOverVault.value = null
  }
}

const onDrop = (event: DragEvent, vault: { name: string }) => {
  dragOverVault.value = null
  if (!props.unlockedVaults.includes(vault.name)) return

  const sourceVault = event.dataTransfer?.getData('sourceVaultName')
  const idsStr = event.dataTransfer?.getData('passedbox/ids')
  
  if (sourceVault && idsStr && sourceVault !== vault.name) {
    try {
      const ids = JSON.parse(idsStr)
      emit('upload-cross-vault', { targetVault: vault.name, sourceVault, ids })
    } catch(e) {}
  }
}

const showNewDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

const toggleDropdown = () => {
  showNewDropdown.value = !showNewDropdown.value
}

const handleClickOutside = (event: MouseEvent) => {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    showNewDropdown.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

const selectVault = (vault: { name: string, path: string }) => {
  emit('select-vault', vault)
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-header">
      <div class="logo">
        <img src="/logo.svg" alt="PassedBox Logo" class="logo-img" />
        <span class="logo-text">PassedBox</span>
      </div>
    </div>

    <!-- Scrollable Content -->
    <div class="sidebar-content">
      <div class="sidebar-nav">
        <div class="nav-group">
          <h3 class="nav-title">My Vaults</h3>
          <ul class="nav-list">
            <li 
              v-for="vault in vaults" 
              :key="vault.path"
              class="nav-item"
              :class="{ 
                active: selectedVaultName === vault.name,
                unlocked: unlockedVaults.includes(vault.name),
                'drag-over': dragOverVault === vault.name
              }"
              @click="selectVault(vault)"
              @dragover.prevent="onDragOver(vault)"
              @dragleave="onDragLeave(vault)"
              @drop.prevent="onDrop($event, vault)"
            >
              <span class="nav-icon">
                <Unlock v-if="unlockedVaults.includes(vault.name)" :size="18" />
                <HardDrive v-else :size="18" />
              </span>
              <span class="nav-label">{{ vault.name }}</span>
            </li>
            
            <!-- New Vault Button at bottom of list -->
             <li class="nav-item create-vault-btn" @click="$emit('create-vault')" title="Create New Vault (Alt+N)">
              <span class="nav-icon"><Plus :size="18" /></span>
              <span class="nav-label">New Vault <kbd>Alt+N</kbd></span>
            </li>
          </ul>
        </div>
      </div>
    </div>

    <div class="sidebar-footer" v-if="appVersion">
      <div class="app-version">Version: {{ appVersion }}</div>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: 256px;
  background-color: var(--bg-surface);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  height: 100%;
  flex-shrink: 0; /* Prevent shrinking */
}

.sidebar-header {
  padding: 1.5rem;
  display: flex;
  align-items: center;
  flex-shrink: 0;
}

.logo {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  font-weight: 600;
  font-size: 1.25rem;
  color: var(--text-main);
}

.logo-img {
  width: 32px;
  height: 32px;
}

.sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding-bottom: 1rem;
}

.create-vault-btn {
    color: var(--primary);
    margin-top: 0.5rem;
    border-top: 1px solid var(--border);
    border-radius: 0;
    padding-top: 1rem;
}

.create-vault-btn:hover {
    background: transparent;
    color: var(--primary-hover);
    text-decoration: underline;
}

.nav-group {
  padding: 0 0.75rem;
}

.nav-title {
  padding: 0 0.75rem;
  font-size: 0.75rem;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--text-muted);
  margin-bottom: 0.5rem;
  letter-spacing: 0.05em;
}

.nav-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.625rem 0.75rem;
  border-radius: 8px;
  color: var(--text-muted);
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s;
}

.nav-item:hover {
  background-color: var(--bg-surface-hover);
  color: var(--text-main);
}

.nav-item.active {
  background-color: rgba(11, 150, 132, 0.15); /* Primary with opacity */
  color: var(--primary);
}

.nav-item.unlocked .nav-icon {
  color: var(--success);
}

.nav-item.active.unlocked .nav-icon {
  color: var(--success);
}

.nav-item kbd {
    margin-left: auto;
    font-size: 0.7rem;
    color: var(--text-faint);
    background: var(--bg-app);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.15rem 0.4rem;
    font-family: inherit;
}

.nav-item.drag-over {
  background-color: rgba(11, 150, 132, 0.3);
  border: 1px dashed var(--primary);
}

.sidebar-footer {
  padding: 1rem;
  border-top: 1px solid var(--border);
}

.app-version {
  font-size: 0.75rem;
  color: var(--text-faint);
  text-align: center;
}

.btn-theme {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.75rem;
  background: transparent;
  border: none;
  color: var(--text-muted);
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-theme:hover {
  background: var(--bg-surface-hover);
  color: var(--text-main);
}

.btn-lock {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 0.75rem;
  background: transparent;
  border: none;
  color: var(--text-muted);
  padding: 0.5rem 0.75rem;
  border-radius: 8px;
  font-size: 0.9rem;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
}

.btn-lock:hover {
  background: rgba(239, 68, 68, 0.1); /* Red tint */
  color: var(--danger);
}
</style>
