<script setup lang="ts">
import { ChevronRight, ClipboardPaste, Copy, Download, FilePlus, FileUp, FolderPlus, FolderUp, Grid, List, Loader2, Lock, Moon, Pencil, Plus, Scissors, Search, Settings, Sun, Trash2, Unlock, X } from 'lucide-vue-next';
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { SearchFiles } from '../../bindings/passedbox/vaultmanager';

const props = defineProps<{
  breadcrumbs: { name: string, path: string }[]
  folderBreadcrumbs: { id: string, name: string }[]
  searchQuery: string
  viewMode: 'grid' | 'list'
  isDark: boolean
  isInVault?: boolean
  isFileView?: boolean
  isSettingsView?: boolean
  isUnlocked?: boolean
  vaultName?: string
  selectedCount: number
  hasClipboard: boolean
}>()

const router = useRouter();

const emit = defineEmits<{
  (e: 'update:searchQuery', value: string): void
  (e: 'navigate', path: string): void
  (e: 'toggle-view'): void
  (e: 'toggle-theme'): void
  (e: 'create-folder'): void
  (e: 'create-file'): void
  (e: 'upload-file'): void
  (e: 'upload-folder'): void
  (e: 'lock-vault'): void
  (e: 'navigate-folder', index: number): void
  (e: 'move-to-folder', payload: { fileId: string, targetFolderId: string }): void
  (e: 'cut-selection'): void
  (e: 'copy-selection'): void
  (e: 'paste-selection'): void
  (e: 'rename-selection'): void
  (e: 'delete-selection'): void
  (e: 'clear-selection'): void
  (e: 'export-selection'): void
  (e: 'select-search-result', result: any): void
}>()

const showNewDropdown = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)
const dragOverTargetId = ref<string | null>(null)
const searchExpanded = ref(false)
const searchInputRef = ref<HTMLInputElement | null>(null)
const searchWrapperRef = ref<HTMLElement | null>(null)

// Search state
const searchResults = ref<any[]>([])
const isSearching = ref(false)
const selectedSearchIndex = ref(-1)
let searchTimeout: ReturnType<typeof setTimeout> | null = null

const expandSearch = () => {
  searchExpanded.value = true
  showNewDropdown.value = false
  nextTick(() => searchInputRef.value?.focus())
}

const collapseSearch = () => {
  setTimeout(() => {
    if (!searchWrapperRef.value?.contains(document.activeElement)) {
      searchResults.value = []
      if (!props.searchQuery) {
        searchExpanded.value = false
      }
    }
  }, 200)
}

watch(() => props.searchQuery, (query) => {
  selectedSearchIndex.value = -1
  if (searchTimeout) clearTimeout(searchTimeout)
  if (!query.trim()) {
    searchResults.value = []
    isSearching.value = false
    return
  }
  isSearching.value = true
  searchTimeout = setTimeout(async () => {
    try {
      const results = await SearchFiles(query)
      searchResults.value = results || []
    } catch {
      searchResults.value = []
    } finally {
      isSearching.value = false
    }
  }, 300)
})

watch(() => props.isUnlocked, (unlocked) => {
  if (!unlocked) {
    searchExpanded.value = false
    searchResults.value = []
  }
})

const handleSearchResultClick = (result: any) => {
  emit('select-search-result', result)
  emit('update:searchQuery', '')
  searchResults.value = []
  searchExpanded.value = false
  selectedSearchIndex.value = -1
}

const handleSearchKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    e.preventDefault()
    emit('update:searchQuery', '')
    searchResults.value = []
    searchExpanded.value = false
    return
  }
  
  if (searchResults.value.length === 0) return

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedSearchIndex.value = (selectedSearchIndex.value + 1) % searchResults.value.length
    scrollToSelected()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedSearchIndex.value = selectedSearchIndex.value <= 0 
      ? searchResults.value.length - 1 
      : selectedSearchIndex.value - 1
    scrollToSelected()
  } else if (e.key === 'Enter') {
    e.preventDefault()
    if (selectedSearchIndex.value >= 0 && selectedSearchIndex.value < searchResults.value.length) {
      handleSearchResultClick(searchResults.value[selectedSearchIndex.value])
    }
  }
}

const scrollToSelected = () => {
  nextTick(() => {
    const activeItem = document.querySelector('.search-item.search-item-active') as HTMLElement
    if (activeItem) {
      activeItem.scrollIntoView({ block: 'nearest' })
    }
  })
}

const formatSearchSize = (bytes: number) => {
  if (!bytes || bytes === 0) return ''
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const toggleDropdown = () => {
  showNewDropdown.value = !showNewDropdown.value
}

const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as Node
  if (dropdownRef.value && !dropdownRef.value.contains(target)) {
    showNewDropdown.value = false
  }
  if (searchWrapperRef.value && !searchWrapperRef.value.contains(target)) {
    if (!props.searchQuery) {
      searchExpanded.value = false
    }
    searchResults.value = []
  }
}

const onCrumbDragOver = (event: DragEvent, targetId: string) => {
  event.preventDefault()
  event.dataTransfer!.dropEffect = 'move'
  dragOverTargetId.value = targetId
}

const onCrumbDragLeave = () => {
  dragOverTargetId.value = null
}

const onCrumbDrop = (event: DragEvent, targetFolderId: string) => {
  event.preventDefault()
  const fileId = event.dataTransfer!.getData('text/plain')
  dragOverTargetId.value = null
  if (!fileId) return
  emit('move-to-folder', { fileId, targetFolderId })
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
})

defineExpose({ expandSearch })
</script>

<template>
  <header class="topbar">
    <div class="topbar-left">

      <!-- New Dropdown (Only in Vault & Unlocked & Not in Settings) -->
      <div v-if="isInVault && isUnlocked && !isSettingsView && !isFileView" class="new-action-wrapper" ref="dropdownRef">
        <button class="icon-btn" @click="toggleDropdown" title="Create New...">
          <Plus :size="28" />
        </button>

        <ul v-if="showNewDropdown" class="dropdown-menu">
          <li @click="$emit('create-file'); showNewDropdown = false">
            <FilePlus :size="18" />
            <span>New File</span>
            <kbd>Ctrl+N</kbd>
          </li>
          <li @click="$emit('create-folder'); showNewDropdown = false">
            <FolderPlus :size="18" />
            <span>New Folder</span>
            <kbd>Ctrl+Shift+N</kbd>
          </li>
          <div class="dropdown-divider"></div>
          <li @click="$emit('upload-file'); showNewDropdown = false">
            <FileUp :size="18" />
            <span>Upload Files</span>
            <kbd>Ctrl+U</kbd>
          </li>
          <li @click="$emit('upload-folder'); showNewDropdown = false">
            <FolderUp :size="18" />
            <span>Upload Folder</span>
            <kbd>Ctrl+Shift+U</kbd>
          </li>
        </ul>
      </div>

      <!-- Lock/Unlock Icon -->
      <div class="vault-status" v-if="isInVault">
        <div 
          class="status-icon" 
          :class="{ 'unlocked': isUnlocked }"
          @click="isUnlocked ? emit('lock-vault') : null"
        >
          <div class="icon-wrapper-inner" :title="isUnlocked ? 'Lock Vault' : 'Vault is Locked'">
            <Unlock v-if="isUnlocked" :size="20" />
            <Lock v-else :size="20" />
            <span v-if="isUnlocked" class="hover-text">Lock Vault</span>
          </div>
        </div>
      </div>

      <div class="breadcrumbs">
        <!-- Vault-level breadcrumbs -->
        <template v-for="(crumb, index) in breadcrumbs" :key="crumb.path">
          <ChevronRight v-if="index > 0 || isInVault" :size="16" class="crumb-separator" />
          
          <span 
            class="crumb"
            :class="{ 
              active: !isFileView && !isSettingsView && index === breadcrumbs.length - 1 && folderBreadcrumbs.length === 0,
              'drag-over': dragOverTargetId === 'root'
            }"
            @click="folderBreadcrumbs.length > 0 ? emit('navigate-folder', -1) : ((isFileView || isSettingsView || index !== breadcrumbs.length - 1) ? emit('navigate', crumb.path) : null)"
            @dragover="onCrumbDragOver($event, 'root')"
            @dragleave="onCrumbDragLeave"
            @drop="onCrumbDrop($event, '')"
          >
            {{ crumb.name }}
          </span>
        </template>

        <!-- Folder breadcrumbs -->
        <template v-for="(crumb, idx) in folderBreadcrumbs" :key="crumb.id">
          <ChevronRight :size="16" class="crumb-separator" />
          <span 
            class="crumb"
            :class="{ 
              active: !isSettingsView && idx === folderBreadcrumbs.length - 1,
              'drag-over': dragOverTargetId === 'crumb-' + crumb.id
            }"
            @click="(isSettingsView || idx !== folderBreadcrumbs.length - 1) ? emit('navigate-folder', idx) : null"
            @dragover="onCrumbDragOver($event, 'crumb-' + crumb.id)"
            @dragleave="onCrumbDragLeave"
            @drop="onCrumbDrop($event, crumb.id)"
          >
            {{ crumb.name }}
          </span>
        </template>
      </div>
    </div>

    <div class="topbar-right">
      <!-- Search -->
      <div v-if="searchExpanded" class="search-wrapper expanded" ref="searchWrapperRef">
        <Search :size="18" class="search-icon" />
        <input 
          ref="searchInputRef"
          type="text" 
          :value="searchQuery"
          @input="emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
          @blur="collapseSearch"
          @keydown="handleSearchKeydown"
          placeholder="Search in vaults..." 
          class="search-input"
        />
        
        <!-- Search Dropdown -->
        <div v-if="searchQuery.trim()" class="search-dropdown">
          <div class="search-header">
            <span class="search-title">Search results</span>
            <span class="search-meta">
              <Loader2 v-if="isSearching" class="animate-spin" :size="14" />
              <span v-else>{{ searchResults.length }} result{{ searchResults.length !== 1 ? 's' : '' }}</span>
            </span>
          </div>
          <div v-if="searchResults.length === 0 && !isSearching" class="search-empty">
            No files or folders found
          </div>
          <div v-else class="search-list">
            <div 
              v-for="(result, index) in searchResults" 
              :key="result.id"
              class="search-item"
              :class="{ 'search-item-active': index === selectedSearchIndex }"
              @mousedown="handleSearchResultClick(result)"
              @mouseenter="selectedSearchIndex = index"
            >
              <div class="search-item-icon" :class="{ folder: result.isFolder }">
                {{ result.isFolder ? '📁' : '📄' }}
              </div>
              <div class="search-item-info">
                <div class="search-item-name">{{ result.name }}</div>
                <div class="search-item-location">
                  <span class="search-vault-tag">{{ result.vaultName }}</span>
                  <span v-if="result.path" class="search-path">{{ result.path }}</span>
                </div>
              </div>
              <div v-if="!result.isFolder && result.size" class="search-item-size">
                {{ formatSearchSize(result.size) }}
              </div>
            </div>
          </div>
        </div>
      </div>
      <button v-else class="icon-btn" @click.stop="expandSearch" title="Search (Ctrl+F or /)">
        <Search :size="28" />
      </button>

      <div class="action-divider"></div>

      <!-- Selection Actions (badge-style, only when items are selected) -->
      <template v-if="selectedCount > 0 && !isFileView">
        <span v-if="selectedCount > 1" class="sel-badge">{{ selectedCount }}</span>
        <button class="icon-btn" @click="emit('cut-selection')" title="Cut (Ctrl+X)">
          <Scissors :size="28" />
        </button>
        <button class="icon-btn" @click="emit('copy-selection')" title="Copy (Ctrl+C)">
          <Copy :size="28" />
        </button>
        <button class="icon-btn" @click="emit('export-selection')" title="Download Selection (Ctrl+S)">
          <Download :size="28" />
        </button>
        <button v-if="hasClipboard" class="icon-btn accent" @click="emit('paste-selection')" title="Paste (Ctrl+V)">
          <ClipboardPaste :size="28" />
        </button>
        <button v-if="selectedCount === 1" class="icon-btn" @click="emit('rename-selection')" title="Rename (F2)">
          <Pencil :size="28" />
        </button>
        <button class="icon-btn danger" @click="emit('delete-selection')" title="Delete">
          <Trash2 :size="28" />
        </button>
        <button class="icon-btn" @click="emit('clear-selection')" title="Clear Selection (Esc)">
          <X :size="28" />
        </button>
        <div class="action-divider"></div>
      </template>

      <!-- Paste hint when clipboard has items but nothing selected -->
      <button v-else-if="hasClipboard && isInVault && isUnlocked" class="icon-btn accent" @click="emit('paste-selection')" title="Paste here (Ctrl+V)">
        <ClipboardPaste :size="28" />
      </button>

      <button v-if="isInVault && isUnlocked && !isFileView && !isSettingsView" class="icon-btn" @click="emit('toggle-view')" title="Toggle View">
        <List v-if="viewMode === 'grid'" :size="28" />
        <Grid v-else :size="28" />
      </button>
      <button class="icon-btn" @click="emit('toggle-theme')" title="Toggle Theme">
        <Moon v-if="isDark" :size="28" />
        <Sun v-else :size="28" />
      </button>
      <button v-if="isInVault && isUnlocked && !isSettingsView" class="icon-btn" @click="router.push(`/vault/${vaultName}/settings`)" title="Vault Settings">
        <Settings :size="28" />
      </button>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 1.5rem;
  background-color: var(--bg-app);
  border-bottom: 1px solid var(--border);
}

.topbar-left {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

.topbar-right {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 0.25rem;
}

/* Selection Badge */
.sel-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  padding: 0 6px;
  border-radius: 11px;
  background: var(--primary);
  color: #fff;
  font-size: 0.75rem;
  font-weight: 700;
  line-height: 1;
}


.action-divider {
  width: 1px;
  height: 24px;
  background: var(--border);
  margin: 0 0.15rem;
}

/* New Dropdown */
.new-action-wrapper {
    position: relative;
}

.dropdown-menu {
    position: absolute;
    top: calc(100% + 8px);
    left: 0;
    min-width: 260px;
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 0.5rem;
    box-shadow: var(--shadow);
    z-index: 100;
    animation: slideDown 0.15s ease-out;
    list-style: none;
    margin: 0;
}

.dropdown-menu li {
    display: flex;    
    align-items: center;
    gap: 0.75rem;
    padding: 0.6rem 0.75rem;
    color: var(--text-main);
    border-radius: 8px;
    cursor: pointer;
    font-size: 0.9rem;
    transition: all 0.15s;
    white-space: nowrap;
}

.dropdown-menu li:hover {
    background: var(--bg-surface-hover);
}

.dropdown-menu li kbd {
    margin-left: auto;
    font-size: 0.7rem;
    color: var(--text-faint);
    background: var(--bg-app);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.15rem 0.4rem;
    font-family: inherit;
}

.dropdown-divider {
  height: 1px;
  background: var(--border);
  margin: 0.25rem 0;
}

/* Vault Status Icon */
.vault-status {
    display: flex;
    align-items: center;
    gap: 0.5rem;
}

.status-icon.unlocked {
    color: var(--success);
    cursor: pointer;
}

.icon-wrapper-inner {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.25rem 0.6rem;
    border-radius: 20px;
    transition: all 0.2s;
    position: relative;
}

.status-icon.unlocked:hover .icon-wrapper-inner {
    background: rgba(16, 185, 129, 0.1);
    color: var(--success);
}

.hover-text {
    font-size: 0.8rem;
    font-weight: 600;
    max-width: 0;
    overflow: hidden;
    white-space: nowrap;
    transition: all 0.3s ease;
    opacity: 0;
}

.status-icon.unlocked:hover .hover-text {
    max-width: 80px;
    opacity: 1;
}

/* Breadcrumbs */
.breadcrumbs {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  color: var(--text-muted);
  font-size: 0.9rem;
  min-width: 0;
  overflow: hidden;
}

.crumb {
  cursor: pointer;
  padding: 0.25rem 0.5rem;
  border-radius: 6px;
  transition: all 0.2s;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.crumb:hover {
  background: var(--bg-surface-hover);
  color: var(--text-main);
}

.crumb.active {
  font-weight: 600;
  color: var(--text-main);
  cursor: default;
}

.crumb.drag-over {
  background: rgba(11, 150, 132, 0.2);
  color: var(--primary);
  outline: 2px dashed var(--primary);
  outline-offset: -2px;
}

.crumb-separator {
  color: var(--text-faint);
  flex-shrink: 0;
}

/* Search */
.search-wrapper {
  position: relative;
}

.search-wrapper.expanded {
  width: 360px;
  animation: searchExpand 0.2s ease-out;
}

@keyframes searchExpand {
  from { width: 50px; opacity: 0; }
  to { width: 360px; opacity: 1; }
}

.search-icon {
  position: absolute;
  left: 1rem;
  top: 50%;
  transform: translateY(-50%);
  color: var(--text-muted);
  pointer-events: none;
}

.search-input {
  width: 100%;
  padding: 0.75rem 1rem 0.75rem 2.75rem;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 24px;
  color: var(--text-main);
  font-size: 0.95rem;
  transition: all 0.2s;
  outline: none;
}

.search-input:focus {
  background: var(--bg-app);
  border-color: var(--primary);
  box-shadow: 0 0 0 3px rgba(11, 150, 132, 0.15);
}

@keyframes slideDown {
  from { opacity: 0; transform: translateY(-10px); }
  to { opacity: 1; transform: translateY(0); }
}

.animate-spin { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

/* Search Dropdown styles */
.search-dropdown {
  position: absolute;
  top: calc(100% + 8px);
  right: 0;
  width: 400px;
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.3);
  z-index: 100;
  overflow: hidden;
  animation: slideDown 0.15s ease-out;
}

.search-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border);
  background: var(--bg-app);
}

.search-title {
  font-weight: 600;
  font-size: 0.85rem;
}

.search-meta {
  color: var(--text-muted);
  font-size: 0.75rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.search-empty {
  padding: 2rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.9rem;
}

.search-list {
  max-height: 400px;
  overflow-y: auto;
}

.search-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.75rem 1rem;
  cursor: pointer;
  transition: background 0.15s;
  border-bottom: 1px solid var(--border);
}

.search-item:last-child {
  border-bottom: none;
}

.search-item:hover, .search-item.search-item-active {
  background: var(--bg-surface-hover);
}

.search-item-icon {
  font-size: 1.25rem;
  flex-shrink: 0;
  width: 24px;
  text-align: center;
}

.search-item-info {
  flex: 1;
  min-width: 0;
}

.search-item-name {
  font-weight: 500;
  font-size: 0.9rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.search-item-location {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 4px;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.search-vault-tag {
  background: rgba(11, 150, 132, 0.1);
  color: var(--primary);
  border: 1px solid rgba(11, 150, 132, 0.3);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 0.65rem;
  font-weight: 600;
  flex-shrink: 0;
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.search-path {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  opacity: 0.8;
}

.search-item-size {
  color: var(--text-muted);
  font-size: 0.75rem;
  flex-shrink: 0;
}
</style>
