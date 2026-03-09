<script setup lang="ts">
import { VueMonacoEditor } from '@guolao/vue-monaco-editor';
import { Dialogs } from '@wailsio/runtime';
import { ArrowLeft, Download, Loader2, Music, Pencil, Save, X } from 'lucide-vue-next';
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue';
import { useRoute, useRouter } from 'vue-router';
import { ExportFile, GetFile, GetFileMetadata, UpdateFileContent } from '../../bindings/passedbox/vaultmanager';
import { useToast } from '../composables/useToast';
import { formatError } from '../utils';

const route = useRoute()
const router = useRouter()
const { addToast } = useToast()

const vaultName = route.params.name as string
const fileId = computed(() => route.params.id as string)

const isLoading = ref(true)
const fileMetadata = ref<any>(null)
const contentUrl = ref('')
const textContent = ref('')
const isDownloading = ref(false)
const forceTextMode = ref(false)

const isEditing = ref(false)
const editContent = ref('')
const isSaving = ref(false)
const monacoEditorRef = ref<any>(null)

const handleEditorMount = (editor: any) => {
    monacoEditorRef.value = editor
}

watch(() => route.params.id, (newId, oldId) => {
    if (newId && newId !== oldId) {
        fileMetadata.value = null
        contentUrl.value = ''
        textContent.value = ''
        isEditing.value = false
        forceTextMode.value = false
        loadFile()
    }
})

const startEditing = () => {
    editContent.value = textContent.value
    isEditing.value = true
    nextTick(() => {
        monacoEditorRef.value?.focus()
    })
}

const saveContent = async () => {
    try {
        isSaving.value = true
        await UpdateFileContent(vaultName, fileId.value, editContent.value)
        textContent.value = editContent.value
        isEditing.value = false
        addToast("File saved successfully", "success")
        fileMetadata.value = await GetFileMetadata(vaultName, fileId.value)
    } catch (e) {
        addToast(formatError(e) || "Failed to save file", "error")
    } finally {
        isSaving.value = false
    }
}

const handleKeydown = (e: KeyboardEvent) => {
    // Don't intercept when a modal is open
    if (document.querySelector('.modal-overlay')) return
    
    if (isEditing.value) {
        // While editing, block ALL shortcuts from propagating to parent handlers
        // Only allow Ctrl+S (save) and Escape (cancel edit)
        if (e.key === 's' && (e.ctrlKey || e.metaKey)) {
            e.preventDefault()
            e.stopPropagation()
            saveContent()
        } else if (e.key === 'Escape') {
            e.preventDefault()
            e.stopPropagation()
            isEditing.value = false
        } else {
            // Block all other shortcuts from reaching parent handlers
            e.stopPropagation()
        }
    } else {
        if ((e.ctrlKey || e.metaKey) && (e.key === 'e' || e.key === 'E')) {
            e.preventDefault()
            startEditing()
        } else if (e.key === 'Escape') {
            e.preventDefault()
            goBack()
        } else if ((e.ctrlKey || e.metaKey) && (e.key === 's' || e.key === 'S')) {
            e.preventDefault()
            handleDownload()
        }
    }
}

const fileType = computed(() => {
    if (forceTextMode.value) return 'text'
    if (!fileMetadata.value) return 'unknown'
    const ext = fileMetadata.value.name.split('.').pop()?.toLowerCase() || ''
    if (['jpg', 'png', 'gif', 'jpeg', 'webp', 'svg', 'bmp'].includes(ext)) return 'image'
    if (['mp4', 'mov', 'avi', 'webm'].includes(ext)) return 'video'
    if (['mp3', 'wav', 'ogg'].includes(ext)) return 'audio'
    if (['txt', 'md', 'doc', 'docx', 'log', 'json', 'js', 'ts', 'go', 'html', 'css', 'yml', 'yaml', 'xml'].includes(ext)) return 'text'
    if (ext === 'pdf') return 'pdf'
    return 'file'
})

const monacoLanguage = computed(() => {
    if (!fileMetadata.value) return 'plaintext'
    const ext = fileMetadata.value.name.split('.').pop()?.toLowerCase() || ''
    
    const langMap: Record<string, string> = {
        'js': 'javascript',
        'jsx': 'javascript',
        'ts': 'typescript',
        'tsx': 'typescript',
        'md': 'markdown',
        'json': 'json',
        'html': 'html',
        'css': 'css',
        'scss': 'scss',
        'less': 'less',
        'xml': 'xml',
        'yaml': 'yaml',
        'yml': 'yaml',
        'go': 'go',
        'py': 'python',
        'java': 'java',
        'c': 'c',
        'cpp': 'cpp',
        'cs': 'csharp',
        'php': 'php',
        'rb': 'ruby',
        'rs': 'rust',
        'sql': 'sql',
        'sh': 'shell',
        'bash': 'shell',
    }
    
    return langMap[ext] || 'plaintext'
})

const isPreviewable = computed(() => {
    return true // Always attempt to load the file, even if unknown to show in Monaco
})

const loadFile = async () => {
    try {
        isLoading.value = true
        
        // 1. Fetch Metadata
        if (!fileMetadata.value) {
            const meta = await GetFileMetadata(vaultName, fileId.value)
            fileMetadata.value = meta
        }

        // 2. Fetch Content if previewable
        if (isPreviewable.value) {
            console.log("Fetching content...")
            const base64Data = await GetFile(vaultName, fileId.value)
            
            // Convert base64 to Blob
            const byteCharacters = atob(base64Data as unknown as string);
            const byteNumbers = new Array(byteCharacters.length);
            for (let i = 0; i < byteCharacters.length; i++) {
                byteNumbers[i] = byteCharacters.charCodeAt(i);
            }
            const byteArray = new Uint8Array(byteNumbers);
            
            let mimeType = 'application/octet-stream';
            // Safe access
            const name = fileMetadata.value?.name || '';
            const ext = name.split('.').pop()?.toLowerCase() || '';
            
            if (forceTextMode.value) mimeType = 'text/plain'; // Force text
            else if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp'].includes(ext)) {
                if (ext === 'svg') mimeType = 'image/svg+xml';
                else if (ext === 'webp') mimeType = 'image/webp';
                else if (ext === 'bmp') mimeType = 'image/bmp';
                else mimeType = `image/${ext === 'jpg' ? 'jpeg' : ext}`;
            }
            else if (['mp4', 'webm'].includes(ext)) mimeType = `video/${ext}`;
            else if (['mp3', 'wav', 'ogg'].includes(ext)) mimeType = `audio/${ext}`;
            else if (ext === 'pdf') mimeType = 'application/pdf';
            else if (['txt', 'md', 'json', 'log', 'js', 'ts', 'go', 'html', 'css', 'yml', 'yaml', 'xml'].includes(ext)) mimeType = 'text/plain';

            const blob = new Blob([byteArray], { type: mimeType });
            
            // Always update textContent so it's ready
            textContent.value = await blob.text()
            if (!isEditing.value) {
                editContent.value = textContent.value
            }
            
            if (fileType.value !== 'text' && !forceTextMode.value) {
                if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
                contentUrl.value = URL.createObjectURL(blob)
            }
        }
    } catch (e) {
        console.error("Failed to load file", e)
        addToast(formatError(e), "error")
        // Optionally redirect back if metadata fails
        // router.push({ name: 'VaultView', params: { name: vaultName } })
    } finally {
        isLoading.value = false
        if (route.query.edit === 'true') {
            startEditing()
        }
    }
}

watch(() => route.query.edit, (newVal) => {
    if (newVal === 'true' && !isLoading.value && !isEditing.value) {
        startEditing()
    }
})

const forceTextView = () => {
    forceTextMode.value = true
    loadFile()
}

const handleDownload = async () => {
    if (!fileMetadata.value) return
    try {
        isDownloading.value = true
        const savePath = await Dialogs.SaveFile({
            Title: "Save File",
            Filename: fileMetadata.value.name,
        })

        let path = ''
        if (typeof savePath === 'string') path = savePath;
        else if (savePath && typeof savePath === 'object') path = (savePath as any).Selection;

        if (path) {
            await ExportFile(vaultName, fileId.value, path)
            addToast("File saved successfully", "success")
        }
    } catch (e) {
        if (!(e as string).includes("cancelled")) {
             addToast(formatError(e), "error")
        }
    } finally {
        isDownloading.value = false
    }
}

const goBack = () => {
    router.push({ name: 'VaultView', params: { name: vaultName } })
}

onMounted(() => {
    loadFile()
    window.addEventListener('keydown', handleKeydown)
})

onUnmounted(() => {
    if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
    window.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="file-view">
    <!-- Header -->
    <header class="view-header">
      <div class="left">
        <button class="btn-icon" @click="goBack" title="Back to Vault (Esc)">
          <ArrowLeft :size="20" />
        </button>
        <div class="file-info" v-if="fileMetadata">
            <h1>{{ fileMetadata.name }}</h1>
            <span class="meta-text">{{ ((fileMetadata.size || 0) / 1024).toFixed(1) }} KB &bull; {{ new Date(fileMetadata.createdAt).toLocaleDateString() }}</span>
        </div>
        <div v-else class="skeleton-header"></div>
      </div>
      <div class="right flex gap-2">
        <template v-if="fileMetadata">
          <button v-if="isEditing" class="btn-icon danger" @click="isEditing = false" title="Cancel (Esc)">
             <X :size="20" />
          </button>
          <button v-if="isEditing" class="btn-icon accent" @click="saveContent" :disabled="isSaving" title="Save (Ctrl+S)">
             <Loader2 v-if="isSaving" class="animate-spin" :size="20" />
             <Save v-else :size="20" />
          </button>
          <button v-else class="btn-icon" @click="startEditing" title="Edit (Ctrl+E)">
             <Pencil :size="20" />
          </button>
        </template>
        <button v-if="!isEditing" class="btn-icon" @click="handleDownload" :disabled="isDownloading || !fileMetadata" title="Download (Ctrl+S)">
          <Loader2 v-if="isDownloading" class="animate-spin" :size="20" />
          <Download v-else :size="20" />
        </button>
      </div>
    </header>

    <!-- Content Area -->
    <main class="view-content">
        <div v-if="isLoading" class="loading-state">
            <Loader2 class="animate-spin" :size="48" />
            <p>Decrypting & Loading...</p>
        </div>

        <div v-else-if="!fileMetadata" class="error-state">
            <p>File not found or could not be loaded.</p>
        </div>

        <template v-else>
            <!-- Image -->
            <div v-if="fileType === 'image' && !isEditing" class="media-preview">
                <img :src="contentUrl" :alt="fileMetadata.name" />
            </div>

            <!-- Video -->
            <div v-else-if="fileType === 'video' && !isEditing" class="media-preview">
                <video :src="contentUrl" controls autoplay></video>
            </div>

            <!-- Audio -->
            <div v-else-if="fileType === 'audio' && !isEditing" class="audio-preview">
                <div class="audio-icon"><Music :size="64" /></div>
                <audio :src="contentUrl" controls autoplay></audio>
            </div>

            <!-- PDF -->
            <div v-else-if="fileType === 'pdf' && !isEditing" class="pdf-preview">
                <iframe :src="contentUrl"></iframe>
            </div>

            <!-- Text / Editor (Default for Text & Unknown) -->
            <div v-else class="text-preview">
                <div class="editor-container">
                     <vue-monaco-editor
                        v-model:value="editContent"
                        theme="vs-dark"
                        :language="monacoLanguage"
                        :options="{
                            automaticLayout: true,
                            minimap: { enabled: false },
                            scrollBeyondLastLine: false,
                            wordWrap: 'on',
                            readOnly: !isEditing
                        }"
                        @mount="handleEditorMount"
                    />
                </div>
            </div>

        </template>
    </main>
  </div>
</template>

<style scoped>
.file-view {
    height: 100vh;
    display: flex;
    flex-direction: column;
    background: var(--bg-app);
}

.view-header {
    height: 64px;
    padding: 0 1.5rem;
    display: flex;
    align-items: center;
    justify-content: space-between;
    background: var(--bg-surface);
    border-bottom: 1px solid var(--border);
}

.left {
    display: flex;
    align-items: center;
    gap: 1rem;
}

.file-info h1 {
    font-size: 1rem;
    font-weight: 600;
    margin: 0;
    color: var(--text-main);
}

.meta-text {
    font-size: 0.75rem;
    color: var(--text-muted);
}

.btn-icon {
    background: transparent;
    border: none;
    color: var(--text-main);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: var(--radius);
    display: flex;
    align-items: center;
    justify-content: center;
}

.btn-icon:hover {
    background: var(--bg-surface-hover);
}

.view-content {
    flex: 1;
    overflow: hidden;
    position: relative;
    background: #0f0f0f;
    display: flex;
    flex-direction: column;
}

.loading-state, .error-state {
    color: var(--text-muted);
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1rem;
}

.media-preview {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
}

img, video {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    border-radius: 4px;
    box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.pdf-preview {
    width: 100%;
    height: 100%;
}

iframe {
    width: 100%;
    height: 100%;
    border: none;
    background: white;
}

.text-preview {
    width: 100%;
    height: 100%;
    overflow: hidden; /* Changed from auto to let monaco handle scroll */
    background: var(--bg-app);
    color: var(--text-main);
    text-align: left; /* Explicitly align left */
    display: flex;
    flex-direction: column;
}

.editor-container {
    flex: 1;
    width: 100%;
    height: 100%;
}

.editor-container :deep(.monaco-editor) {
    padding-top: 1rem;
}

.audio-preview {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2rem;
}

.animate-spin {
    animation: spin 1s linear infinite;
}

@keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
}

.mt-4 { margin-top: 1rem; }
.flex { display: flex; }
.gap-4 { gap: 1rem; }

.btn-secondary {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    color: var(--text-main);
    white-space: nowrap;
    padding: 0.5rem 1rem;
    min-width: max-content;
    flex-shrink: 0;
}
.btn-secondary:hover {
    background: var(--bg-surface-hover);
    border-color: var(--primary);
}
</style>
