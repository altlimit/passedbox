<script setup lang="ts">
import { Dialogs, Events } from '@wailsio/runtime'
import { Loader2 } from 'lucide-vue-next'
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterView, useRoute, useRouter } from 'vue-router'
import { GetAppVersion } from '../bindings/passedbox/appservice'
import { CancelAction, CopyAcrossVaults, CreateFile, CreateFolder, CreateVault, DeleteFiles, ExportFiles, GetDevicePepperInfo, ImportFiles, ImportFolder, ImportPaths, ListVaults, LockVault, MoveFile, RenameFile } from '../bindings/passedbox/vaultmanager'
import ProgressBar from './components/ProgressBar.vue'
import Sidebar from './components/Sidebar.vue'
import ToastContainer from './components/ToastContainer.vue'
import TopBar from './components/TopBar.vue'
import { useClipboard } from './composables/useClipboard'
import { useToast } from './composables/useToast'
import { formatError } from './utils'

const { addToast } = useToast()
const router = useRouter()
const route = useRoute()

// Global State
const vaults = ref<{id: string, name: string, path: string}[]>([])
const unlockedVaults = ref<string[]>([])
const searchQuery = ref('')
// Removed redundant search block variables
const isLoading = ref(true)
const viewMode = ref<'grid' | 'list'>('grid')
const refreshKey = ref(0) // To trigger re-fetch in VaultView
const vaultViewRef = ref<any>(null)
const topBarRef = ref<any>(null)
const folderBreadcrumbs = ref<{id: string, name: string}[]>([])
const vaultBreadcrumbsMap = ref<Record<string, {id: string, name: string}[]>>({})
const appVersion = ref('')

const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0
const isModKey = (e: KeyboardEvent | MouseEvent) => isMac ? e.metaKey : e.ctrlKey

// Progress State
const progressState = ref({
  active: false,
  op: '',
  percent: 0,
  message: ''
})

// Create Vault Modal State
const showCreateModal = ref(false)
const newVaultName = ref('')
const newVaultPassword = ref('')
const newVaultUseDevicePepper = ref(false)
const devicePepperInfo = ref<{available: boolean, serialId: string, isRemovable: boolean} | null>(null)
const isCreating = ref(false)
const vaultNameInput = ref<HTMLInputElement | null>(null)
const newVaultPasswordInput = ref<HTMLInputElement | null>(null)

watch(showCreateModal, async (val) => {
  if (val) {
    newVaultUseDevicePepper.value = false
    try {
      devicePepperInfo.value = await GetDevicePepperInfo()
    } catch (e) {
      console.error("Failed to get device info", e)
    }
    
    nextTick(() => {
      vaultNameInput.value?.focus()
    })
  }
})

// Create Folder Modal State
const showFolderModal = ref(false)
const newFolderName = ref('')
const folderNameInput = ref<HTMLInputElement | null>(null)

// Create File Modal State
const showFileModal = ref(false)
const newFileName = ref('')
const fileNameInput = ref<HTMLInputElement | null>(null)

// Rename Modal State
const showRenameModal = ref(false)
const renameValue = ref('')
const renameFileId = ref('')
const renameInputRef = ref<HTMLInputElement | null>(null)

// Confirm Modal State
const showConfirmModal = ref(false)
const confirmMessage = ref('')
const confirmAction = ref<(() => void) | null>(null)
const confirmYesBtn = ref<HTMLButtonElement | null>(null)
const confirmNoBtn = ref<HTMLButtonElement | null>(null)

const confirmSettle = (result: boolean) => {
  showConfirmModal.value = false
  if (result && confirmAction.value) {
    confirmAction.value()
  }
  confirmAction.value = null
}

const showConfirm = (message: string, onConfirm: () => void) => {
  confirmMessage.value = message
  confirmAction.value = onConfirm
  showConfirmModal.value = true
  nextTick(() => {
    confirmYesBtn.value?.focus()
  })
}

// Selection State
const selectedCount = ref(0)
const selectedIds = ref<string[]>([])
const { clipboard } = useClipboard()
const hasClipboard = computed(() => !!clipboard.value)

const currentVaultName = computed(() => route.params.name as string)
const currentVault = computed(() => vaults.value.find(v => v.name === currentVaultName.value))
const isInVault = computed(() => !!currentVaultName.value)

watch(currentVaultName, (newName, oldName) => {
  if (oldName && oldName !== newName) {
    vaultBreadcrumbsMap.value[oldName] = [...folderBreadcrumbs.value]
  }
  if (newName) {
    folderBreadcrumbs.value = vaultBreadcrumbsMap.value[newName] || []
  } else {
    folderBreadcrumbs.value = []
  }
})
const breadcrumbs = computed(() => {
  const path = route.path
  if (path === '/') return []
  const parts = path.split('/').filter(Boolean)
  
  // If viewing a file, remove 'vault' prefix and only show [VaultName] -> [File]
  // Route: /vault/:name/file/:id
  if (route.name === 'FileView') {
      const vaultIndex = parts.indexOf('vault')
      if (vaultIndex !== -1 && parts[vaultIndex + 1]) {
          // Return [vaultName] (skip 'vault')
          const meaningfulParts = parts.slice(vaultIndex + 1, vaultIndex + 2) // Just vault name for now
           return meaningfulParts.map((part) => ({
              name: part,
              path: '/vault/' + part 
          }))
      }
  }

  // General Case: Filter out 'vault' keyword
  return parts
    .filter(part => part !== 'vault')
    .map((part) => {
        const originalIndex = parts.indexOf(part)
        return {
            name: part,
            path: '/' + parts.slice(0, originalIndex + 1).join('/')
        }
    })
})

// Theme Management
const isDark = ref(true)
const toggleTheme = () => {
  isDark.value = !isDark.value
  updateTheme()
}

const updateTheme = () => {
  document.documentElement.classList.toggle('dark', isDark.value)
  if (isDark.value) {
    document.documentElement.removeAttribute('data-theme')
  } else {
    document.documentElement.setAttribute('data-theme', 'light')
  }
}

// Vault Management
const fetchVaults = async () => {
  try {
    isLoading.value = true
    const vaultsList = await ListVaults()
    vaults.value = vaultsList.map((v: any) => ({ id: v.id, name: v.name, path: v.path }))
    
    // Auto-select if only 1 vault and on home screen
    if (vaults.value.length && route.path === '/') {
      router.push(`/vault/${vaults.value[0].name}`)
    }
  } catch (e) {
    console.error(e)
    addToast('Failed to load vaults', 'error')
  } finally {
    isLoading.value = false
  }
}

const handleVaultSelection = (vault: { name: string, path: string }) => {
  router.push(`/vault/${vault.name}`)
}

const handleVaultUnlocked = (vaultName: string) => {
  if (!unlockedVaults.value.includes(vaultName)) {
    unlockedVaults.value.push(vaultName)
  }
}

const handleVaultLock = async () => {
  if (currentVaultName.value) {
    try {
      await LockVault(currentVaultName.value)
      unlockedVaults.value = unlockedVaults.value.filter(v => v !== currentVaultName.value)
      addToast(`Locked ${currentVaultName.value}`, 'success')
      // Make sure search and breadcrumbs drop out
      searchQuery.value = ''
      folderBreadcrumbs.value = []
      if (currentVaultName.value) {
        vaultBreadcrumbsMap.value[currentVaultName.value] = []
      }
      router.push(`/vault/${currentVaultName.value}`)
    } catch (e) {
      addToast(formatError(e) || 'Failed to lock vault', 'error')
    }
  }
}

const handleCreate = async () => {
  if (!newVaultName.value || !newVaultPassword.value) {
    addToast('Please fill in all fields', 'error')
    return
  }
  
  try {
    isCreating.value = true
    await CreateVault(newVaultName.value, newVaultPassword.value, newVaultUseDevicePepper.value)
    addToast('Vault created successfully', 'success')
    showCreateModal.value = false
    newVaultName.value = ''
    newVaultPassword.value = ''
    newVaultUseDevicePepper.value = false
    await fetchVaults()
  } catch (e: any) {
    addToast(formatError(e), 'error')
  } finally {
    isCreating.value = false
  }
}

const handleGlobalKeydown = (e: KeyboardEvent) => {
  const tag = (e.target as HTMLElement).tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA') {
      if (isModKey(e) && (e.key === 'l' || e.key === 'L')) {
          // allow lock from anywhere
      } else if (!(!unlockedVaults.value.includes(currentVaultName.value) && !showCreateModal.value && (e.target as HTMLInputElement).type === 'password')) {
          return // don't intercept unless it's locked vault password input
      }
  }

  // When viewing/editing a file, only allow lock vault shortcut
  if (route.name === 'FileView') {
      if (isModKey(e) && (e.key === 'l' || e.key === 'L')) {
          e.preventDefault()
          handleVaultLock()
      }
      return
  }

  // Lock Vault shortcut
  if (isModKey(e) && (e.key === 'l' || e.key === 'L')) {
      e.preventDefault()
      handleVaultLock()
      return
  }

  // New Vault shortcut (Alt + N)
  if (e.altKey && !e.shiftKey && (e.key === 'n' || e.key === 'N')) {
      e.preventDefault()
      showCreateModal.value = true
      return
  }

  // Cross-vault navigation shortcut (Alt + Up/Down)
  if (e.altKey && !e.shiftKey && (e.key === 'ArrowUp' || e.key === 'ArrowDown')) {
      e.preventDefault()
      if (vaults.value.length === 0) return
      
      let nextIdx = vaults.value.findIndex(v => v.name === currentVaultName.value)
      if (nextIdx === -1) nextIdx = 0
      
      if (e.key === 'ArrowDown') {
          nextIdx = Math.min(vaults.value.length - 1, nextIdx + 1)
      } else {
          nextIdx = Math.max(0, nextIdx - 1)
      }
      
      const vault = vaults.value[nextIdx]
      if (vault && vault.name !== currentVaultName.value) {
          router.push(`/vault/${vault.name}`)
      }
      return
  }

  if (isModKey(e) && (e.key === 'f' || e.key === 'F')) {
    // Only expand search if in an unlocked vault
    if (unlockedVaults.value.includes(currentVaultName.value)) {
      e.preventDefault()
      topBarRef.value?.expandSearch()
    }
    return
  }
}

onMounted(() => {
  GetAppVersion().then(v => {
      appVersion.value = v
  })
  fetchVaults()
  // Initialize theme
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
    isDark.value = true
  }
  updateTheme()
  window.addEventListener('keydown', handleGlobalKeydown)

  // Progress Listener
  const unlisten = Events.On('progress', (event: any) => {
    const data = Array.isArray(event.data) ? event.data[0] : event.data;
    progressState.value = {
      active: true,
      op: data.op,
      percent: data.percent,
      message: data.message
    }
    
    // Auto-hide when complete AND the queue is empty
    if (data.percent >= 100 && uploadQueue.value.length === 0) {
      setTimeout(() => {
        if (progressState.value.percent >= 100 && uploadQueue.value.length === 0) {
          progressState.value.active = false
        }
      }, 1500)
    }
  })

  // Drag and Drop (Wails side fallback natively if needed)
  const unlistenDrop = Events.On('files-dropped', (event: any) => {
    console.log("dropped", event)
    if (!currentVaultName.value) return;
    const data = event.data;
    let paths: string[] = [];
    if (data?.paths) {
      paths = data.paths;
    }
    if (paths && paths.length > 0) {
       const parentID = (data.target.indexOf('folder_') === 0 ? data.target.substring(7) : vaultViewRef.value?.currentParentID) || ''
       uploadQueue.value.push({ type: 'mixed', vault: currentVaultName.value, parentID, paths })
       processUploadQueue()
    }
  })
  ;(window as any)._unlistenDrop = unlistenDrop;

  // Hack for unmounted cleanup of events if needed, though onUnmounted is below
  ;(window as any)._unlistenProgress = unlisten
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleGlobalKeydown)
  if ((window as any)._unlistenProgress) {
    (window as any)._unlistenProgress()
  }
  if ((window as any)._unlistenDrop) {
    (window as any)._unlistenDrop()
  }
})

const handleCreateFolder = () => {
  newFolderName.value = ''
  showFolderModal.value = true
  nextTick(() => folderNameInput.value?.focus())
}

const confirmCreateFolder = async () => {
  if (!newFolderName.value.trim() || !currentVaultName.value) return
  try {
    const parentID = vaultViewRef.value?.currentParentID || ''
    await CreateFolder(currentVaultName.value, parentID, newFolderName.value.trim())
    addToast(`Folder "${newFolderName.value.trim()}" created`, 'success')
    showFolderModal.value = false
    newFolderName.value = ''
    vaultViewRef.value?.fetchFiles()
  } catch (e) {
    addToast(formatError(e) || 'Failed to create folder', 'error')
  }
}

const handleCreateFile = () => {
  newFileName.value = 'Untitled.txt'
  showFileModal.value = true
  nextTick(() => {
    if (fileNameInput.value) {
      fileNameInput.value.focus()
      const dotIndex = newFileName.value.lastIndexOf('.')
      if (dotIndex > 0) {
        fileNameInput.value.setSelectionRange(0, dotIndex)
      } else {
        fileNameInput.value.select()
      }
    }
  })
}

const confirmCreateFile = async () => {
  if (!newFileName.value.trim() || !currentVaultName.value) return
  try {
    const parentID = vaultViewRef.value?.currentParentID || ''
    const fileID = await CreateFile(currentVaultName.value, parentID, newFileName.value.trim())
    addToast(`File "${newFileName.value.trim()}" created`, 'success')
    showFileModal.value = false
    newFileName.value = ''
    vaultViewRef.value?.fetchFiles() // Refresh list just in case
    
    // Navigate to edit mode
    router.push(`/vault/${currentVaultName.value}/file/${fileID}?edit=true`)
  } catch (e) {
    addToast(formatError(e) || 'Failed to create file', 'error')
  }
}

const handleFolderChanged = (parentId: string, crumbs: {id: string, name: string}[]) => {
  folderBreadcrumbs.value = [...crumbs]
  if (currentVaultName.value) {
    vaultBreadcrumbsMap.value[currentVaultName.value] = [...crumbs]
  }
}

const handleNavigateFolder = (index: number) => {
  if (route.name === 'FileView' || route.name === 'VaultSettings') {
    // We are viewing a file, so VaultView methods aren't available.
    // Slice globally and navigate back to the Vault root; the initialBreadcrumbs
    // prop will handle restoring the state when VaultView mounts.
    if (index === -1) {
      folderBreadcrumbs.value = []
    } else {
      folderBreadcrumbs.value = folderBreadcrumbs.value.slice(0, index + 1)
    }
    router.push('/vault/' + currentVaultName.value)
  } else if (vaultViewRef.value?.navigateToBreadcrumb) {
    vaultViewRef.value.navigateToBreadcrumb(index)
  }
}

const handleMoveToFolder = async (payload: { fileId: string, targetFolderId: string }) => {
  if (!currentVaultName.value) return
  try {
    await MoveFile(currentVaultName.value, payload.fileId, payload.targetFolderId)
    refreshKey.value++
  } catch (e) {
    addToast(formatError(e) || 'Failed to move item', 'error')
  }
}

const handleSelectionChanged = (payload: { count: number, ids: string[] }) => {
  if (payload.count === -1) {
    // Delete signal
    handleDeleteSelection(payload.ids)
    return
  }
  if (payload.count === -2) {
    // Rename signal
    handleRenameSelection(payload.ids[0])
    return
  }
  if (payload.count === -3) {
    // Create folder signal
    handleCreateFolder()
    return
  }
  if (payload.count === -4) {
    // Upload signal
    handleUploadFile()
    return
  }
  if (payload.count === -5) {
    handleUploadFolder()
    return
  }
  if (payload.count === -6) {
    handleExportSelection()
    return
  }
  selectedCount.value = payload.count
  selectedIds.value = payload.ids
}

const handleCutSelection = () => {
  vaultViewRef.value?.cutSelection()
}

const handleCopySelection = () => {
  vaultViewRef.value?.copySelection()
}

const handlePasteSelection = () => {
  vaultViewRef.value?.pasteSelection()
}

const handleClearSelection = () => {
  vaultViewRef.value?.clearSelection()
  selectedCount.value = 0
  selectedIds.value = []
}

const handleRenameSelection = async (fileId?: string) => {
  const id = fileId || (selectedIds.value.length === 1 ? selectedIds.value[0] : null)
  if (!id) return
  // Find current name from VaultView
  const files = vaultViewRef.value?.files
  const file = files?.find((f: any) => f.raw.id === id)
  const oldName = file?.name || ''
  
  renameFileId.value = id
  renameValue.value = oldName
  showRenameModal.value = true
  nextTick(() => {
    if (renameInputRef.value) {
      renameInputRef.value.focus()
      const dotIndex = oldName.lastIndexOf('.')
      if (dotIndex > 0) {
        renameInputRef.value.setSelectionRange(0, dotIndex)
      } else {
        renameInputRef.value.select()
      }
    }
  })
}

const confirmRename = async () => {
  const id = renameFileId.value
  const newName = renameValue.value.trim()
  if (!id || !newName) return

  // Find old name to skip no-op
  const files = vaultViewRef.value?.files
  const file = files?.find((f: any) => f.raw.id === id)
  if (file && newName === file.name) {
    showRenameModal.value = false
    return
  }

  try {
    await RenameFile(currentVaultName.value, id, newName)
    addToast('Renamed successfully', 'success')
    showRenameModal.value = false
    vaultViewRef.value?.fetchFiles()
  } catch (e) {
    addToast(formatError(e) || 'Failed to rename', 'error')
  }
}

const handleDeleteSelection = async (ids?: string[]) => {
  const idsToDelete = ids || [...selectedIds.value]
  if (!currentVaultName.value || idsToDelete.length === 0) return

  const message = idsToDelete.length === 1 
    ? 'Are you sure you want to delete this item? This action cannot be undone.'
    : `Are you sure you want to delete ${idsToDelete.length} items? This action cannot be undone.`

  showConfirm(message, async () => {
    try {
      handleClearSelection()
      await DeleteFiles(currentVaultName.value, idsToDelete)
      vaultViewRef.value?.fetchFiles()
    } catch (e) {
      addToast(formatError(e) || 'Failed to delete items', 'error')
    }
  })
}

// --- Upload Queue System ---
const uploadQueue = ref<{ type: 'file' | 'folder' | 'files' | 'mixed', vault: string, parentID: string, path?: string, paths?: string[] }[]>([])
const isProcessingQueue = ref(false)

const processUploadQueue = async () => {
  if (isProcessingQueue.value) return
  isProcessingQueue.value = true

  try {
    while (uploadQueue.value.length > 0) {
      const task = uploadQueue.value[0]
      
      try {
        if (task.type === 'files' && task.paths) {
          await ImportFiles(task.vault, task.parentID, task.paths)
        } else if (task.type === 'file' && task.path) {
          await ImportFiles(task.vault, task.parentID, [task.path])
        } else if (task.type === 'folder' && task.path) {
          await ImportFolder(task.vault, task.parentID, task.path)
        } else if (task.type === 'mixed' && task.paths) {
          await ImportPaths(task.vault, task.parentID, task.paths)
        }
      } catch (e) {
        if (!(e as string || '').includes('cancelled')) {
           addToast(formatError(e), 'error')
        }
      }

      uploadQueue.value.shift()
      // Optional: short delay between items if needed
    }
  } finally {
    isProcessingQueue.value = false
    vaultViewRef.value?.fetchFiles()
    setTimeout(() => {
      // If queue is empty and the final operation reached 100%, hide it
      if (uploadQueue.value.length === 0 && progressState.value.percent >= 100) {
        progressState.value.active = false
      }
    }, 1500)
  }
}

const handleUploadFile = async () => {
  if (!currentVaultName.value) return
  
  try {
    const result = await Dialogs.OpenFile({
        Title: "Select Files to Upload",
        CanChooseFiles: true,
        AllowsMultipleSelection: true,
        Filters: [
            { DisplayName: "All Files", Pattern: "*.*" }
        ]
    })
    
    const res = result as any
    let paths: string[] = []
    
    if (typeof res === 'string' && res) {
        paths = [res]
    } else if (Array.isArray(res)) {
        paths = res.filter((p: any) => typeof p === 'string' && p)
    } else if (res && typeof res === 'object' && 'Selection' in res) {
        const sel = res.Selection
        if (typeof sel === 'string') paths = [sel]
        else if (Array.isArray(sel)) paths = sel
    }

    if (paths.length > 0) {
         const parentID = vaultViewRef.value?.currentParentID || ''
         uploadQueue.value.push({ type: 'files', vault: currentVaultName.value, parentID, paths })
         processUploadQueue()
    }
  } catch (e) {
     if (!(e as string || '').includes('cancelled')) {
        addToast(formatError(e), 'error')
     }
  }
}

const handleUploadFolder = async () => {
  if (!currentVaultName.value) return
  
  try {
    const result = await Dialogs.OpenFile({
        Title: "Select Folder to Upload",
        CanChooseDirectories: true,
        CanChooseFiles: false,
        CanCreateDirectories: false,
        ShowHiddenFiles: false
    })
    
    const res = result as any
    let path = ''
    if (typeof res === 'string' && res) {
      path = res
    } else if (res && typeof res === 'object' && 'Selection' in res) {
      if (typeof res.Selection === 'string') {
        path = res.Selection
      }
    }
    
    if (path) {
      const parentID = vaultViewRef.value?.currentParentID || ''
      uploadQueue.value.push({ type: 'folder', vault: currentVaultName.value, parentID, path })
      processUploadQueue()
    }
    
  } catch (e) {
    if (!(e as string).includes('cancelled')) {
        addToast(formatError(e), 'error')
    }
  }
}

const handleCrossVaultUpload = async (payload: { targetVault: string, sourceVault: string, ids: string[] }) => {
  try {
    await CopyAcrossVaults(payload.sourceVault, payload.targetVault, '', payload.ids)
    
    // Refresh if we are viewing the target vault already
    if (currentVaultName.value === payload.targetVault) {
      vaultViewRef.value?.fetchFiles()
    }
  } catch (e) {
    addToast(formatError(e) || 'Failed to copy items', 'error')
  }
}

const handleExportSelection = async () => {
  if (!currentVaultName.value) return
  const ids = Array.from((vaultViewRef.value as any)?.selectedIds || []) as string[]
  if (ids.length === 0) return

  try {
    const result = await Dialogs.OpenFile({
      Title: "Select Download Destination",
      CanChooseDirectories: true,
      CanChooseFiles: false,
      CanCreateDirectories: true,
      ShowHiddenFiles: false
    })
    const res = result as any
    let destPath = ''
    if (typeof res === 'string' && res) {
      destPath = res
    } else if (res && typeof res === 'object' && 'Selection' in res) {
      if (typeof res.Selection === 'string') {
        destPath = res.Selection
      }
    }

    if (destPath) {
      progressState.value = {
        active: true,
        op: 'export',
        percent: 0,
        message: 'Preparing download...'
      }
      await ExportFiles(currentVaultName.value, destPath, ids)
      // Clear selection after download
      vaultViewRef.value?.clearSelection()
      addToast('Download completed successfully', 'success')
    }
  } catch (e) {
    if (!(e as string).includes('cancelled')) {
      addToast(formatError(e), 'error')
    }
  }
}

const handleCancelOperation = async (op: string) => {
  showConfirm(`Are you sure you want to cancel the active ${op} operation?`, async () => {
    try {
      await CancelAction();
      addToast('Operation cancelled', 'info');
      uploadQueue.value = [];
      progressState.value.active = false;
    } catch (e) {
      addToast('Failed to cancel operation: ' + String(e), 'error');
    }
  })
}

// Searching handled mostly in TopBar now

const handleSearchResultClick = (result: any) => {
  searchQuery.value = ''
  
  let fullPathNodes = result.pathNodes || []
  if (result.isFolder) {
    fullPathNodes = [...fullPathNodes, { id: result.id, name: result.name }]
  }
  
  folderBreadcrumbs.value = fullPathNodes

  if (result.isFolder) {
    if (currentVaultName.value === result.vaultName) {
      vaultViewRef.value?.navigateViaSearch?.(result.id, folderBreadcrumbs.value)
    } else {
      // Navigate to vault first so VaultView mounts
      vaultBreadcrumbsMap.value[result.vaultName] = [...folderBreadcrumbs.value]
      router.push('/vault/' + result.vaultName)
      // Then tell VaultView to load this specific folder
      nextTick(() => {
        setTimeout(() => {
          vaultViewRef.value?.navigateViaSearch?.(result.id, folderBreadcrumbs.value)
        }, 300)
      })
    }
  } else {
    // Open file preview (file view will use the global folderBreadcrumbs for the TopBar)
    router.push('/vault/' + result.vaultName + '/file/' + result.id)
  }
}

</script>

<template>
  <div class="app-layout">
    <ProgressBar v-bind="progressState" :queueLength="uploadQueue.length" @cancel="handleCancelOperation" />
    <ToastContainer />
    <!-- Sidebar -->
    <Sidebar 
      :vaults="vaults" 
      :selected-vault-name="currentVaultName"
      :unlocked-vaults="unlockedVaults"
      :is-dark="isDark"
      :is-in-vault="isInVault"
      :app-version="appVersion"
      @select-vault="handleVaultSelection"
      @create-vault="showCreateModal = true"
      @lock-vault="handleVaultLock"
      @upload-cross-vault="handleCrossVaultUpload"
    />

    <!-- Main Content Area -->
    <main class="main-area">
      <TopBar 
        ref="topBarRef"
        :breadcrumbs="breadcrumbs" 
        :folder-breadcrumbs="folderBreadcrumbs"
        v-model:searchQuery="searchQuery" 
        :view-mode="viewMode"
        :is-dark="isDark"
        :is-in-vault="isInVault"
        :is-file-view="route.name === 'FileView'"
        :is-settings-view="route.name === 'VaultSettings'"
        :is-unlocked="unlockedVaults.includes(currentVaultName)"
        :vault-name="currentVaultName"
        :selected-count="selectedCount"
        :has-clipboard="hasClipboard"
        @navigate="(path) => router.push(path)"
        @toggle-view="viewMode = viewMode === 'grid' ? 'list' : 'grid'"
        @toggle-theme="toggleTheme"
        @create-folder="handleCreateFolder"
        @create-file="handleCreateFile"
        @upload-file="handleUploadFile"
        @upload-folder="handleUploadFolder"
        @lock-vault="handleVaultLock"
        @navigate-folder="handleNavigateFolder"
        @move-to-folder="handleMoveToFolder"
        @cut-selection="handleCutSelection"
        @copy-selection="handleCopySelection"
        @paste-selection="handlePasteSelection"
        @rename-selection="handleRenameSelection"
        @delete-selection="handleDeleteSelection"
        @clear-selection="handleClearSelection"
        @export-selection="handleExportSelection"
        @select-search-result="handleSearchResultClick"
      />
      


      <div class="content-wrapper">
         <div v-if="isLoading" class="flex items-center justify-center h-full text-muted"> <Loader2 class="animate-spin" /> </div>
         <div v-else-if="vaults.length === 0" class="empty-layout">
            <h3>Welcome to PassedBox</h3>
            <p>Create your first vault to get started.</p>
            <button class="btn-primary mt-4" @click="showCreateModal = true">Create Vault</button>
         </div>
         
         <RouterView v-else v-slot="{ Component }">
            <component 
              :is="Component" 
              ref="vaultViewRef"
              :vault-name="currentVaultName"
              :vault-id="currentVault?.id"
              :is-unlocked="unlockedVaults.includes(currentVaultName)"
              :view-mode="viewMode"
              :refresh-key="refreshKey"
              :initial-breadcrumbs="folderBreadcrumbs"
              @unlocked="handleVaultUnlocked"
              @folder-changed="handleFolderChanged"
              @selection-changed="handleSelectionChanged"              
            />
         </RouterView>
      </div>
    </main>

    <!-- Create Vault Modal (Global) -->
    <div v-if="showCreateModal" class="modal-overlay" @click.self="showCreateModal = false">
      <div class="modal">
        <h2>Create New Vault</h2>
        <div class="form-group mt-4">
          <label>Name</label>
          <input ref="vaultNameInput" v-model="newVaultName" placeholder="Vault Name" @keyup.enter="handleCreate" />
        </div>
        <div class="form-group">
          <label>Password</label>
          <input ref="newVaultPasswordInput" v-model="newVaultPassword" type="password" placeholder="Master Password" @keyup.enter="handleCreate" />
        </div>
        
        <div class="form-group checkbox-group" v-if="devicePepperInfo?.available">
          <label class="checkbox-label" :title="devicePepperInfo.isRemovable ? 'Ties this vault password to this removable drive' : 'Ties this vault password to this drive'">
            <input type="checkbox" v-model="newVaultUseDevicePepper" />
            Hardware-lock to SN: {{ devicePepperInfo.serialId }}
          </label>
        </div>

        <!-- <p v-if="createError" class="error-text">{{ createError }}</p> -->
        <div class="actions">
           <button @click="showCreateModal = false" class="btn-ghost">Cancel</button>
           <button @click="handleCreate" class="btn-primary" :disabled="isCreating">
             Create
           </button>
        </div>
      </div>
    </div>

    <!-- Create Folder Modal -->
    <div v-if="showFolderModal" class="modal-overlay" @click.self="showFolderModal = false">
      <div class="modal">
        <h2>New Folder</h2>
        <div class="form-group mt-4">
          <label>Folder Name</label>
          <input 
            ref="folderNameInput"
            v-model="newFolderName" 
            placeholder="Untitled folder" 
            @keyup.enter="confirmCreateFolder"
          />
        </div>
        <div class="actions">
           <button @click="showFolderModal = false" class="btn-ghost">Cancel</button>
           <button @click="confirmCreateFolder" class="btn-primary" :disabled="!newFolderName.trim()">Create</button>
        </div>
      </div>
    </div>

    <!-- Create File Modal -->
    <div v-if="showFileModal" class="modal-overlay" @click.self="showFileModal = false">
      <div class="modal">
        <h2>New File</h2>
        <div class="form-group mt-4">
          <label>File Name</label>
          <input 
            ref="fileNameInput"
            v-model="newFileName" 
            placeholder="Untitled.txt" 
            @keyup.enter="confirmCreateFile"
          />
        </div>
        <div class="actions">
           <button @click="showFileModal = false" class="btn-ghost">Cancel</button>
           <button @click="confirmCreateFile" class="btn-primary" :disabled="!newFileName.trim()">Create</button>
        </div>
      </div>
    </div>

    <!-- Rename Modal -->
    <div v-if="showRenameModal" class="modal-overlay" @click.self="showRenameModal = false">
      <div class="modal">
        <h2>Rename</h2>
        <div class="form-group mt-4">
          <label>New Name</label>
          <input 
            ref="renameInputRef"
            v-model="renameValue" 
            placeholder="Enter new name" 
            @keyup.enter="confirmRename"
            @keydown.esc="showRenameModal = false"
          />
        </div>
        <div class="actions">
           <button @click="showRenameModal = false" class="btn-ghost">Cancel</button>
           <button @click="confirmRename" class="btn-primary" :disabled="!renameValue.trim()">Rename</button>
        </div>
      </div>
    </div>

    <!-- Confirm Modal -->
    <div v-if="showConfirmModal" class="modal-overlay" @click.self="confirmSettle(false)" @keydown.esc="confirmSettle(false)">
      <div class="modal">
        <h2>Confirm</h2>
        <p class="modal-desc mt-4">{{ confirmMessage }}</p>
        <div class="actions">
           <button 
             ref="confirmYesBtn" 
             @click="confirmSettle(true)" 
             @keydown.right.prevent="confirmNoBtn?.focus()" 
             @keydown.left.prevent="confirmNoBtn?.focus()" 
             class="btn-primary"
           >Yes</button>
           <button 
             ref="confirmNoBtn" 
             @click="confirmSettle(false)" 
             @keydown.left.prevent="confirmYesBtn?.focus()" 
             @keydown.right.prevent="confirmYesBtn?.focus()" 
             class="btn-ghost"
           >No</button>
        </div>
      </div>
    </div>

  </div>
</template>

<style scoped>
.app-layout {
  display: flex;
  height: 100vh;
  width: 100%;
  background: var(--bg-app);
  color: var(--text-main);
  overflow: hidden;
}

.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.content-wrapper {
  flex: 1;
  overflow: hidden;
  position: relative;
}

.empty-layout {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
}

.animate-spin { animation: spin 1s linear infinite; }
.text-muted { color: var(--text-muted); }

/* Reusing Modal Styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: var(--bg-overlay);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 50;
}
.modal {
  background: var(--bg-surface);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 2rem;
  width: 400px;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
}
.actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  margin-top: 1.5rem;
}
.form-group { margin-bottom: 1rem; }
.form-group label { display: block; margin-bottom: 0.5rem; font-size: 0.8rem; color: var(--text-muted); text-transform: uppercase; }
.modal-desc {
  color: var(--text-muted);
  font-size: 0.9rem;
  margin-top: 0.75rem;
  line-height: 1.5;
}

.btn-primary.mt-4 {
  margin-top: 1rem;
}
@keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }


</style>
