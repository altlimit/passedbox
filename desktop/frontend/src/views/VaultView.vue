<script setup lang="ts">
import { Check, Copy, File, FileText, Folder, Image, Loader2, Lock, Music, Shield, Unlock, Video } from 'lucide-vue-next';
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { CheckDMSRelease, CopyAcrossVaults, CopyFiles, ListFiles, MoveAcrossVaults, MoveFile, UnlockVault, UnlockVaultWithShare3 } from '../../bindings/passedbox/vaultmanager';
import { useClipboard } from '../composables/useClipboard';
import { useToast } from '../composables/useToast';
import { copyToClipboard, formatError } from '../utils';

const { addToast } = useToast();
const props = defineProps<{
  vaultName: string
  vaultId: string
  isUnlocked: boolean
  viewMode: 'grid' | 'list'
  refreshKey?: number
  initialBreadcrumbs?: {id: string, name: string}[]
}>()

const emit = defineEmits<{
  (e: 'unlocked', vaultName: string): void
  (e: 'folder-changed', parentId: string, breadcrumbs: {id: string, name: string}[]): void
  (e: 'selection-changed', payload: { count: number, ids: string[] }): void
}>()

const router = useRouter()
const password = ref('')
const error = ref('') 
const isLoading = ref(false)
const isFetchingFiles = ref(false)
const isCopiedId = ref(false)
const passwordInput = ref<HTMLInputElement | null>(null)
const files = ref<any[]>([])

// DMS recovery state
const dmsStatus = ref<{enabled: boolean, released: boolean, releasedAt: string} | null>(null)
const dmsChecking = ref(false)
const dmsRecovering = ref(false)

// Folder navigation state
const currentParentID = ref('')
const folderBreadcrumbs = ref<{id: string, name: string}[]>([])

// Drag state
const draggedFileId = ref<string | null>(null)
const dragOverTargetId = ref<string | null>(null)

// Selection state
const selectedIds = ref<Set<string>>(new Set())
const lastClickedIndex = ref<number>(-1)
const cursorIndex = ref<number>(-1)

// Cut/Paste clipboard
const { clipboard, setClipboard, clearClipboard } = useClipboard()

const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0

const isModKey = (e: KeyboardEvent | MouseEvent) => isMac ? e.metaKey : e.ctrlKey

const emitSelection = () => {
  emit('selection-changed', { count: selectedIds.value.size, ids: [...selectedIds.value] })
}

const clearSelection = () => {
  selectedIds.value = new Set()
  lastClickedIndex.value = -1
  cursorIndex.value = -1
  emitSelection()
}

onMounted(() => {
    if (props.initialBreadcrumbs && props.initialBreadcrumbs.length > 0) {
        folderBreadcrumbs.value = [...props.initialBreadcrumbs]
        currentParentID.value = folderBreadcrumbs.value[folderBreadcrumbs.value.length - 1].id
        emit('folder-changed', currentParentID.value, folderBreadcrumbs.value)
    }
    if (props.isUnlocked) {
        fetchFiles()
    }
})

const selectAll = () => {
  selectedIds.value = new Set(files.value.map(f => f.raw.id))
  emitSelection()
}

const handleFileClick = (event: MouseEvent, file: any, index: number) => {
  cursorIndex.value = index
  if (event.shiftKey && lastClickedIndex.value >= 0) {
    // Range select
    const start = Math.min(lastClickedIndex.value, index)
    const end = Math.max(lastClickedIndex.value, index)
    const newSet = new Set(selectedIds.value)
    for (let i = start; i <= end; i++) {
      newSet.add(files.value[i].raw.id)
    }
    selectedIds.value = newSet
  } else if (isModKey(event)) {
    // Toggle select
    const newSet = new Set(selectedIds.value)
    if (newSet.has(file.raw.id)) {
      newSet.delete(file.raw.id)
    } else {
      newSet.add(file.raw.id)
    }
    selectedIds.value = newSet
  } else {
    // Single select
    selectedIds.value = new Set([file.raw.id])
  }
  lastClickedIndex.value = index
  emitSelection()
}

const handleBackgroundClick = (event: MouseEvent) => {
  // Only clear if clicking directly on the explorer-content div
  if ((event.target as HTMLElement).classList.contains('explorer-content') ||
      (event.target as HTMLElement).classList.contains('file-grid') ||
      (event.target as HTMLElement).classList.contains('file-list')) {
    // Don't clear if clicking on grid/list gap areas
    const target = event.target as HTMLElement
    if (target.classList.contains('explorer-content')) {
      clearSelection()
    }
  }
}

// Cut & Paste & Copy
const cutSelection = () => {
  if (selectedIds.value.size === 0) return
  setClipboard({
    ids: [...selectedIds.value],
    sourceParentId: currentParentID.value,
    sourceVaultName: props.vaultName,
    mode: 'cut'
  })
  emitSelection()
}

const copySelection = () => {
  if (selectedIds.value.size === 0) return
  setClipboard({
    ids: [...selectedIds.value],
    sourceParentId: currentParentID.value,
    sourceVaultName: props.vaultName,
    mode: 'copy'
  })
  emitSelection()
}

const pasteSelection = async () => {
  if (!clipboard.value || clipboard.value.ids.length === 0) return
  
  try {
    const isCrossVault = clipboard.value.sourceVaultName !== props.vaultName

    if (clipboard.value.mode === 'copy') {
      if (isCrossVault) {
        await CopyAcrossVaults(clipboard.value.sourceVaultName, props.vaultName, currentParentID.value, clipboard.value.ids)
      } else {
        await CopyFiles(props.vaultName, currentParentID.value, clipboard.value.ids)
        addToast('Copied items successfully', 'success')
      }
    } else {
      if (isCrossVault) {
        await MoveAcrossVaults(clipboard.value.sourceVaultName, props.vaultName, currentParentID.value, clipboard.value.ids)
      } else {
        for (const id of clipboard.value.ids) {
          await MoveFile(props.vaultName, id, currentParentID.value)
        }
        addToast('Moved items successfully', 'success')
      }
      // Only clear if cut since it was a permanent move
      clearClipboard()
    }
    
    clearSelection()
    fetchFiles()
  } catch (e) {
    addToast(formatError(e) || 'Failed to paste items', 'error')
  }
}

const navigateToFolder = (folderId: string, folderName: string) => {
    currentParentID.value = folderId
    folderBreadcrumbs.value.push({ id: folderId, name: folderName })
    clearSelection()
    emit('folder-changed', folderId, folderBreadcrumbs.value)
    fetchFiles()
}

const navigateViaSearch = (targetFolderId: string, pathNodes: {id: string, name: string}[]) => {
    currentParentID.value = targetFolderId
    folderBreadcrumbs.value = [...pathNodes]
    clearSelection()
    emit('folder-changed', targetFolderId, folderBreadcrumbs.value)
    fetchFiles()
}

const navigateToBreadcrumb = (index: number) => {
    if (index === -1) {
        currentParentID.value = ''
        folderBreadcrumbs.value = []
    } else {
        const crumb = folderBreadcrumbs.value[index]
        currentParentID.value = crumb.id
        folderBreadcrumbs.value = folderBreadcrumbs.value.slice(0, index + 1)
    }
    clearSelection()
    emit('folder-changed', currentParentID.value, folderBreadcrumbs.value)
    fetchFiles()
}

const goBack = () => {
    if (folderBreadcrumbs.value.length === 0) return
    folderBreadcrumbs.value.pop()
    const lastCrumb = folderBreadcrumbs.value[folderBreadcrumbs.value.length - 1]
    currentParentID.value = lastCrumb ? lastCrumb.id : ''
    clearSelection()
    emit('folder-changed', currentParentID.value, folderBreadcrumbs.value)
    fetchFiles()
}

const openViewer = (file: any) => {
    if (file.type === 'folder') {
        navigateToFolder(file.raw.id, file.name)
        return
    }
    router.push({ 
        name: 'FileView', 
        params: { 
            name: props.vaultName, 
            id: file.raw.id 
        } 
    })
}

const fetchFiles = async () => {
  if (!props.isUnlocked || !props.vaultName) return
  
  try {
    isFetchingFiles.value = true
    const result = await ListFiles(props.vaultName, currentParentID.value)
    
    files.value = result.map((f: any) => ({
      name: f.name,
      type: f.isFolder ? 'folder' : getType(f.name),
      size: f.isFolder ? '-' : formatSize(f.size),
      modified: new Date(f.createdAt).toLocaleDateString(),
      raw: f
    }))
    files.value.sort((a: any, b: any) => {
      if (a.type === 'folder' && b.type !== 'folder') return -1
      if (a.type !== 'folder' && b.type === 'folder') return 1
      return a.name.localeCompare(b.name)
    })
  } catch (e) {
    console.error("Failed to fetch files", e)
    addToast('Failed to load files', 'error')
  } finally {
    isFetchingFiles.value = false
  }
}

// Expose for parent
defineExpose({ currentParentID, navigateToFolder, navigateViaSearch, navigateToBreadcrumb, clearSelection, cutSelection, copySelection, pasteSelection, clipboard, selectedIds, fetchFiles, files })

const getType = (name: string) => {
   const ext = name.split('.').pop()?.toLowerCase() || ''
   if (['jpg', 'png', 'gif', 'jpeg'].includes(ext)) return 'image'
   if (['mp4', 'mov', 'avi'].includes(ext)) return 'video'
   if (['mp3', 'wav', 'ogg'].includes(ext)) return 'audio'
   if (['txt', 'md', 'doc', 'pdf', 'docx'].includes(ext)) return 'document'
   return 'file'
}

const formatSize = (bytes: number) => {
   if (bytes === 0) return '0 B'
   const k = 1024
   const sizes = ['B', 'KB', 'MB', 'GB']
   const i = Math.floor(Math.log(bytes) / Math.log(k))
   return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const handleCopyId = async () => {
  if (!props.vaultId) return
  const success = await copyToClipboard(props.vaultId)
  if (success) {
    isCopiedId.value = true
    setTimeout(() => {
      isCopiedId.value = false
    }, 2000)
  }
}

const handleUnlock = async () => {
  if (!password.value) return
  isLoading.value = true
  
  try {
    await UnlockVault(props.vaultName, password.value)
    emit('unlocked', props.vaultName)
    addToast('Vault unlocked successfully!', 'success');
    password.value = ''
    error.value = ''
    // Avoid double-fetching if mounted hook already fetched due to initialBreadcrumbs
    if (!props.initialBreadcrumbs?.length) {
        fetchFiles()
    }
  } catch (e) {
    const msg = formatError(e) || "Incorrect password"
    addToast(msg, 'error')
    password.value = '' 
    nextTick(() => passwordInput.value?.focus())
  }
  isLoading.value = false
}

const checkDMSStatus = async () => {
  dmsStatus.value = null
  dmsChecking.value = true
  try {
    const status = await CheckDMSRelease(props.vaultName)
    dmsStatus.value = status
  } catch (e) {
    console.error('DMS check failed:', e)
  }
  dmsChecking.value = false
}

const handleRecoverVault = async () => {
  dmsRecovering.value = true
  error.value = ''
  try {
    await UnlockVaultWithShare3(props.vaultName)
    emit('unlocked', props.vaultName)
    addToast('Vault recovered via Dead Man\'s Switch!', 'success')
    if (!props.initialBreadcrumbs?.length) {
      fetchFiles()
    }
  } catch (e) {
    error.value = formatError(e) || 'Failed to recover vault'
    addToast(error.value, 'error')
  }
  dmsRecovering.value = false
}

const getFileIcon = (name: string, type: string) => {
  if (type === 'folder') return Folder
  if (type === 'image') return Image
  if (type === 'video') return Video
  if (type === 'audio') return Music
  if (type === 'document') return FileText
  return File
}

// ====== Drag & Drop ======
const onDragStart = (event: DragEvent, id: string) => {
    draggedFileId.value = id
    // Only drag files for now
    if (event.dataTransfer) {
        event.dataTransfer.setData('text/plain', id)
        // Check if selected, if so drag all selected, else drag just this one
        const ids = selectedIds.value.has(id) ? [...selectedIds.value] : [id]
        event.dataTransfer.setData('passedbox/ids', JSON.stringify(ids))
        // Also set source parent so we don't drop on itself
        event.dataTransfer.setData('sourceParentId', currentParentID.value)
        // Pass along sourceVaultName
        event.dataTransfer.setData('sourceVaultName', props.vaultName)
    }
}

const onDragOver = (event: DragEvent, targetId: string) => {
  event.preventDefault()
  event.dataTransfer!.dropEffect = 'move'
  dragOverTargetId.value = targetId
}

const onDragLeave = () => {
  dragOverTargetId.value = null
}

const onDrop = async (event: DragEvent, targetFolderId: string) => {
  event.preventDefault()
  dragOverTargetId.value = null
  const fileId = draggedFileId.value || event.dataTransfer!.getData('text/plain')
  draggedFileId.value = null
  
  if (!fileId || fileId === targetFolderId) return
  
  try {
    await MoveFile(props.vaultName, fileId, targetFolderId)
    fetchFiles()
  } catch (e) {
    addToast(formatError(e) || 'Failed to move item', 'error')
  }
}

const onDragEnd = () => {
  draggedFileId.value = null
  dragOverTargetId.value = null
}

// ====== Keyboard Shortcuts ======
const getCols = () => {
    if (props.viewMode === 'list') return 1
    const grid = document.querySelector('.file-grid')
    if (!grid || grid.children.length === 0) return 1
    const firstOffset = (grid.children[0] as HTMLElement).offsetTop
    let cols = 0
    for(let i=0; i<grid.children.length; i++) {
       if ((grid.children[i] as HTMLElement).offsetTop === firstOffset) cols++
       else break
    }
    return cols || 1
}

const handleKeydown = (e: KeyboardEvent) => {
  if (!props.isUnlocked) return
  
  // Don't intercept when typing in inputs
  const tag = (e.target as HTMLElement).tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') return
  
  // Don't intercept when a modal is open
  if (document.querySelector('.modal-overlay')) return
  
  // Ctrl+Shift+N = New folder
  if (isModKey(e) && e.shiftKey && (e.key === 'N' || e.key === 'n')) {
    e.preventDefault()
    emit('selection-changed', { count: -3, ids: [] }) // -3 signals create folder
    return
  }
  // Ctrl+Shift+U = Upload folder
  if (isModKey(e) && e.shiftKey && (e.key === 'U' || e.key === 'u')) {
    e.preventDefault()
    emit('selection-changed', { count: -5, ids: [] }) // -5 signals upload folder
    return
  }
  // Ctrl+U = Upload file
  if (isModKey(e) && !e.shiftKey && (e.key === 'u' || e.key === 'U')) {
    e.preventDefault()
    emit('selection-changed', { count: -4, ids: [] }) // -4 signals upload
    return
  }
  // Ctrl+S = Download selection
  if (isModKey(e) && (e.key === 's' || e.key === 'S')) {
    e.preventDefault()
    if (selectedIds.value.size > 0) {
      emit('selection-changed', { count: -6, ids: [] })
    }
    return
  }
  
  // Arrow keys navigation
  if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) {
      e.preventDefault()
      if (files.value.length === 0) return
          let nextIndex = cursorIndex.value !== -1 ? cursorIndex.value : 0
          
          if (cursorIndex.value !== -1) {
              if (e.key === 'ArrowLeft') {
                  nextIndex = Math.max(0, cursorIndex.value - 1)
              } else if (e.key === 'ArrowRight') {
                  nextIndex = Math.min(files.value.length - 1, cursorIndex.value + 1)
              } else if (e.key === 'ArrowUp') {
                  nextIndex = Math.max(0, cursorIndex.value - getCols())
              } else if (e.key === 'ArrowDown') {
                  nextIndex = Math.min(files.value.length - 1, cursorIndex.value + getCols())
              }
          }
          
          cursorIndex.value = nextIndex
          const file = files.value[nextIndex]
          
          if (e.shiftKey) {
              if (lastClickedIndex.value === -1) lastClickedIndex.value = nextIndex
              const start = Math.min(lastClickedIndex.value, nextIndex)
              const end = Math.max(lastClickedIndex.value, nextIndex)
              const newSet = new Set<string>()
              for (let i = start; i <= end; i++) {
                  newSet.add(files.value[i].raw.id)
              }
              selectedIds.value = newSet
          } else {
              selectedIds.value = new Set([file.raw.id])
              lastClickedIndex.value = nextIndex
          }
          
          emitSelection()
          nextTick(() => {
              const el = document.getElementById(`${file.type}_${file.raw.id}`)
              if (el) el.scrollIntoView({ block: 'nearest' })
          })
          return
  }

  // Enter to open
  if (e.key === 'Enter') {
      e.preventDefault()
      if (cursorIndex.value >= 0 && cursorIndex.value < files.value.length) {
          openViewer(files.value[cursorIndex.value])
      }
      return
  }

  // Backspace to go back
  if (e.key === 'Backspace') {
      e.preventDefault()
      goBack()
      return
  }
  
  if (isModKey(e) && e.key === 'a') {
    e.preventDefault()
    selectAll()
  } else if (isModKey(e) && e.key === 'x') {
    e.preventDefault()
    cutSelection()
  } else if (isModKey(e) && e.key === 'c') {
    e.preventDefault()
    copySelection()
  } else if (isModKey(e) && e.key === 'v') {
    e.preventDefault()
    pasteSelection()
  } else if (e.key === 'F2') {
    if (selectedIds.value.size === 1) {
      e.preventDefault()
      emit('selection-changed', { count: -2, ids: [...selectedIds.value] }) // -2 signals rename
    }
  } else if (e.key === 'Escape') {
    clearSelection()
  } else if (e.key === 'Delete' || (isModKey(e) && e.key === 'Backspace')) {
    if (selectedIds.value.size > 0) {
      e.preventDefault()
      emit('selection-changed', { count: -1, ids: [...selectedIds.value] }) // -1 signals delete
    }
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown)
  if (!props.isUnlocked) {
    nextTick(() => passwordInput.value?.focus())
    checkDMSStatus()
  } else {
    fetchFiles()
  }
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
})

watch(() => props.vaultName, () => {
  password.value = ''
  error.value = ''
  files.value = []
  if (props.initialBreadcrumbs && props.initialBreadcrumbs.length > 0) {
    folderBreadcrumbs.value = [...props.initialBreadcrumbs]
    currentParentID.value = folderBreadcrumbs.value[folderBreadcrumbs.value.length - 1].id
  } else {
    currentParentID.value = ''
    folderBreadcrumbs.value = []
  }
  clearSelection()
  if (!props.isUnlocked) {
    nextTick(() => passwordInput.value?.focus())
    checkDMSStatus()
  } else {
    fetchFiles()
  }
})

watch(() => props.isUnlocked, (newVal) => {
   if (newVal) {
     fetchFiles()
   } else {
     password.value = ''
     error.value = ''
     currentParentID.value = ''
     folderBreadcrumbs.value = []
     clearSelection()
     nextTick(() => passwordInput.value?.focus())
    checkDMSStatus()
   }
})

watch(() => props.refreshKey, () => {
  if (props.isUnlocked) fetchFiles()
})
</script>

<template>
  <div class="vault-view">
    <!-- LOCKED STATE -->
    <div v-if="!isUnlocked" class="locked-container">
      <div class="unlock-card">
        <div class="icon-circle">
          <Lock :size="32" />
        </div>
        <h2>Unlock {{ vaultName }}</h2>
        <div class="vault-id-container">
          <p class="vault-id-hint">ID: {{ vaultId }}</p>
          <button class="id-copy-btn" @click="handleCopyId" :title="isCopiedId ? 'Copied!' : 'Copy Vault ID'">
            <Check v-if="isCopiedId" :size="12" class="success-icon" />
            <Copy v-else :size="12" />
          </button>
        </div>

        <!-- DMS Released Notice -->
        <div v-if="dmsStatus?.released" class="dms-release-notice">
          <div class="dms-release-header">
            <Shield :size="20" />
            <span>Dead Man's Switch Triggered</span>
          </div>
          <p>This vault can be recovered without a password.</p>
          <button @click="handleRecoverVault" class="btn-primary recover-btn" :disabled="dmsRecovering">
            <Loader2 v-if="dmsRecovering" class="animate-spin" :size="16"/>
            <Shield v-else :size="16" />
            <span>{{ dmsRecovering ? 'Recovering...' : 'Recover Vault' }}</span>
          </button>
          <div class="dms-divider">
            <span>or unlock with password</span>
          </div>
        </div>

        <!-- DMS Checking indicator -->
        <div v-else-if="dmsChecking" class="dms-checking">
          <Loader2 class="animate-spin" :size="14" />
          <span>Checking Dead Man's Switch status...</span>
        </div>

        <p v-else>Enter your master password to access files.</p>
        
        <div class="input-group">
          <input 
            ref="passwordInput"
            v-model="password" 
            type="password" 
            placeholder="Master Password" 
            @keyup.enter="handleUnlock"
            :disabled="isLoading"
          />
          <button @click="handleUnlock" class="btn-primary" :disabled="isLoading">
            <Loader2 v-if="isLoading" class="animate-spin" :size="20" />
            <Unlock v-else :size="20" />
          </button>
        </div>
      </div>
    </div>

    <!-- UNLOCKED STATE (Explorer) -->
    <div v-else class="explorer-content" @click="handleBackgroundClick" data-file-drop-target>

      <div v-if="isFetchingFiles" class="loading-state">
         <Loader2 class="animate-spin text-muted" :size="32" />
      </div>

      <div v-else-if="files.length === 0" class="empty-state">
         <Folder :size="48" class="text-muted mb-4" />
         <p>{{ folderBreadcrumbs.length > 0 ? 'This folder is empty.' : 'This vault is empty.' }}</p>
      </div>

      <template v-else>
        <!-- Grid View -->
        <div v-if="viewMode === 'grid'" class="file-grid">
           <div 
             v-for="(file, index) in files" 
             :key="file.raw.id" 
             class="file-card"
             :class="{ 
               'drag-over': file.type === 'folder' && dragOverTargetId === file.raw.id,
               'dragging': draggedFileId === file.raw.id,
               'selected': selectedIds.has(file.raw.id),
               'cut': clipboard?.ids.includes(file.raw.id)
             }"
             :draggable="true"
             @dragstart="onDragStart($event, file.raw.id)"
             @dragend="onDragEnd"
             @dragover="file.type === 'folder' ? onDragOver($event, file.raw.id) : undefined"
             @dragleave="onDragLeave"
             @drop="file.type === 'folder' ? onDrop($event, file.raw.id) : undefined"
             @click.stop="handleFileClick($event, file, index)"
             @dblclick="openViewer(file)"
             :data-file-drop-target="file.type === 'folder' ? '' : null"
             :id="`${file.type}_${file.raw.id}`"
           >
              <div class="preview">
                 <component :is="getFileIcon(file.name, file.type)" :size="48" :class="file.type === 'folder' ? 'text-primary' : 'text-muted'" />
              </div>
              <div class="details">
                 <span class="name" :title="file.name">{{ file.name }}</span>
                 <span class="meta">{{ file.type === 'folder' ? 'Folder' : file.size }}</span>
              </div>
           </div>
        </div>

        <!-- List View -->
        <div v-else class="file-list">
          <div class="list-header">
            <span class="col-name">Name</span>
            <span class="col-type">Type</span>
            <span class="col-size">Size</span>
            <span class="col-date">Date Modified</span>
          </div>
          <div 
            v-for="(file, index) in files" 
            :key="file.raw.id" 
            class="list-row"
            :class="{ 
              'drag-over': file.type === 'folder' && dragOverTargetId === file.raw.id,
              'dragging': draggedFileId === file.raw.id,
              'selected': selectedIds.has(file.raw.id),
              'cut': clipboard?.ids.includes(file.raw.id)
            }"
            :draggable="true"
            @dragstart="onDragStart($event, file.raw.id)"
            @dragend="onDragEnd"
            @dragover="file.type === 'folder' ? onDragOver($event, file.raw.id) : undefined"
            @dragleave="onDragLeave"
            @drop="file.type === 'folder' ? onDrop($event, file.raw.id) : undefined"
            @click.stop="handleFileClick($event, file, index)"
            @dblclick="openViewer(file)"
            :data-file-drop-target="file.type === 'folder' ? '' : null"
            :id="`${file.type}_${file.raw.id}`"
          >
             <div class="col-name cell-name">
                <component :is="getFileIcon(file.name, file.type)" :size="20" :class="file.type === 'folder' ? 'text-primary' : 'text-muted'" />
                <span :title="file.name">{{ file.name }}</span>
             </div>
             <span class="col-type">{{ file.type === 'folder' ? 'Folder' : 'File' }}</span>
             <span class="col-size">{{ file.size }}</span>
             <span class="col-date">{{ file.modified }}</span>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.vault-view {
  height: 100%;
  width: 100%;
}

/* Locked State */
.locked-container {
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-app);
}

.unlock-card {
  text-align: center;
  max-width: 320px;
  width: 100%;
}

.icon-circle {
  width: 64px;
  height: 64px;
  background: var(--bg-surface);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 1.5rem;
  color: var(--text-muted);
}

.unlock-card p {
  color: var(--text-muted);
  font-size: 0.95rem;
  margin-bottom: 0;
}

.vault-id-hint {
  font-family: monospace;
  font-size: 0.75rem !important;
  opacity: 0.6;
}

.vault-id-container {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.4rem;
  margin-bottom: 0.75rem !important;
}

.id-copy-btn {
  background: none;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 4px;
  padding: 0.2rem;
  color: var(--text-muted);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}

.id-copy-btn:hover {
  border-color: var(--primary);
  color: var(--primary);
  background-color: rgba(var(--primary-rgb), 0.1);
}

.success-icon {
  color: #10b981;
}

.input-group {
  display: flex;
  gap: 0.5rem;
  margin-top: 1.5rem;
}

.input-group input {
  flex: 1;
}

.dms-release-notice {
  background: rgba(245, 158, 11, 0.08);
  border: 1px solid rgba(245, 158, 11, 0.25);
  border-radius: 0.75rem;
  padding: 1rem;
  margin-top: 1rem;
  text-align: left;
}

.dms-release-header {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  color: #f59e0b;
  font-weight: 600;
  font-size: 0.9rem;
  margin-bottom: 0.5rem;
}

.dms-release-notice p {
  font-size: 0.82rem;
  color: var(--text-muted);
  margin-bottom: 0.75rem;
  line-height: 1.4;
}

.recover-btn {
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
}

.dms-divider {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin-top: 1rem;
  color: var(--text-muted);
  font-size: 0.75rem;
}

.dms-divider::before,
.dms-divider::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border);
}

.dms-checking {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5rem;
  font-size: 0.8rem;
  color: var(--text-muted);
  margin-top: 0.5rem;
}

.animate-spin { animation: spin 1s linear infinite; }
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

/* Explorer State */
.explorer-content {
  padding: 1.5rem;
  height: 100%;
  overflow-y: auto;
}

.loading-state, .empty-state {
   height: 100%;
   display: flex;
   flex-direction: column;
   align-items: center;
   justify-content: center;
   color: var(--text-muted);
}

.loading-state {
   color: var(--primary);
}

.mb-4 { margin-bottom: 1rem; }

/* Grid View */
.file-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 1rem;
}

.file-card {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 1rem;
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s;
  user-select: none;
}

.file-card:hover {
  background: var(--bg-surface-hover);
  border-color: var(--border-hover);
  transform: translateY(-2px);
}

/* Selection States */
.file-card.selected,
.list-row.selected {
  background: rgba(11, 150, 132, 0.12);
  border-color: var(--primary);
}

.file-card.selected:hover,
.list-row.selected:hover {
  background: rgba(11, 150, 132, 0.2);
}

/* Cut State */
.file-card.cut,
.list-row.cut {
  opacity: 0.45;
}

/* Drag & Drop States */
.file-card.drag-over,
.list-row.drag-over {
  background: rgba(11, 150, 132, 0.15);
  border-color: var(--primary);
  outline: 2px dashed var(--primary);
  outline-offset: -2px;
  transform: scale(1.02);
}

.file-card.dragging,
.list-row.dragging {
  opacity: 0.4;
  transform: scale(0.95);
}

.preview {
  margin-bottom: 0.75rem;
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.text-primary { color: var(--primary); }
.text-muted { color: var(--text-muted); }

.details {
  width: 100%;
}

.name {
  display: block;
  font-size: 0.9rem;
  font-weight: 500;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-bottom: 0.25rem;
}

.meta {
  font-size: 0.75rem;
  color: var(--text-faint);
}

/* List View */
.file-list {
  display: flex;
  flex-direction: column;
}

.list-header {
  display: flex;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border);
  color: var(--text-muted);
  font-size: 0.85rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.list-row {
  display: flex;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border);
  cursor: pointer;
  transition: all 0.15s;
  user-select: none;
}

.list-row:hover {
  background-color: var(--bg-surface-hover);
}

.col-name { flex: 2; display: flex; align-items: center; gap: 0.75rem; overflow: hidden; }
.col-type { flex: 1; color: var(--text-muted); font-size: 0.9rem; }
.col-size { flex: 0.8; color: var(--text-muted); font-size: 0.9rem; }
.col-date { flex: 1; color: var(--text-muted); font-size: 0.9rem; text-align: right; }

.cell-name span {
  font-weight: 500;
  color: var(--text-main);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
