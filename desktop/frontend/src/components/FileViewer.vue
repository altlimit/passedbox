<script setup lang="ts">
import { Dialogs } from '@wailsio/runtime';
import { Download, File, X } from 'lucide-vue-next';
import { computed, onUnmounted, ref, watch } from 'vue';
import { ExportFile, GetFile } from '../../bindings/passedbox/vaultmanager';
import { useToast } from '../composables/useToast';
import { formatError } from '../utils';

const props = defineProps<{
  isOpen: boolean
  file: any
  vaultName: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const { addToast } = useToast()
const isLoading = ref(false)
const contentUrl = ref('')
const textContent = ref('')
const isDownloading = ref(false)

const fileType = computed(() => {
    if (!props.file) return 'unknown'
    return props.file.type || 'file'
})

const isPreviewable = computed(() => {
    return ['image', 'video', 'audio', 'text', 'document'].includes(fileType.value)
})

const loadContent = async () => {
    if (!props.file || !props.isOpen || !isPreviewable.value) return

    try {
        isLoading.value = true
        // GetFile returns []byte which Wails marshals as base64 string
        const base64Data = await GetFile(props.vaultName, props.file.raw.id)
        
        // Convert base64 to Blob
        const byteCharacters = atob(base64Data as unknown as string);
        const byteNumbers = new Array(byteCharacters.length);
        for (let i = 0; i < byteCharacters.length; i++) {
            byteNumbers[i] = byteCharacters.charCodeAt(i);
        }
        const byteArray = new Uint8Array(byteNumbers);
        let mimeType = 'application/octet-stream';
        
        const ext = props.file.name.split('.').pop()?.toLowerCase()
        if (['jpg', 'jpeg', 'png', 'gif', 'webp', 'svg', 'bmp'].includes(ext)) {
            if (ext === 'svg') mimeType = 'image/svg+xml';
            else if (ext === 'webp') mimeType = 'image/webp';
            else if (ext === 'bmp') mimeType = 'image/bmp';
            else mimeType = `image/${ext === 'jpg' ? 'jpeg' : ext}`;
        }
        else if (['mp4', 'webm'].includes(ext)) mimeType = `video/${ext}`;
        else if (['mp3', 'wav', 'ogg'].includes(ext)) mimeType = `audio/${ext}`;
        else if (ext === 'pdf') mimeType = 'application/pdf';
        else if (['txt', 'md', 'json', 'log', 'js', 'ts', 'go', 'html', 'css'].includes(ext)) mimeType = 'text/plain';

        const blob = new Blob([byteArray], { type: mimeType });
        
        if (fileType.value === 'text' || (fileType.value === 'document' && ext !== 'pdf')) {
            textContent.value = await blob.text()
        } else {
            if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
            contentUrl.value = URL.createObjectURL(blob)
        }

    } catch (e) {
        console.error("Failed to load file content", e)
        addToast("Failed to load preview", "error")
    } finally {
        isLoading.value = false
    }
}

const handleDownload = async () => {
    try {
        isDownloading.value = true
        const savePath = await Dialogs.SaveFile({
            Title: "Save File",
            Filename: props.file.name,
        })

        // Handle Wails v3 return type (string or object)
        let path = ''
        if (typeof savePath === 'string') path = savePath;
        else if (savePath && typeof savePath === 'object') path = (savePath as any).Selection;

        if (path) {
            await ExportFile(props.vaultName, props.file.raw.id, path)
            addToast("File saved successfully", "success")
        }
    } catch (e) {
        if ((e as string) !== 'cancelled') {
             addToast(formatError(e), "error")
        }
    } finally {
        isDownloading.value = false
    }
}

const close = () => {
    if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
    contentUrl.value = ''
    textContent.value = ''
    emit('close')
}

watch(() => props.isOpen, (newVal) => {
    if (newVal) loadContent()
    else {
        if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
        contentUrl.value = ''
        textContent.value = ''
    }
})

// Cleanup
onUnmounted(() => {
    if (contentUrl.value) URL.revokeObjectURL(contentUrl.value)
})

</script>

<template>
  <div v-if="isOpen" class="viewer-overlay" @click.self="close">
    <div class="viewer-container">
      <div class="viewer-header">
         <h3>{{ file.name }}</h3>
         <div class="actions">
            <button class="btn-icon" @click="handleDownload" title="Download">
                <Download :size="20" />
            </button>
            <button class="btn-icon" @click="close" title="Close">
                <X :size="20" />
            </button>
         </div>
      </div>

      <div class="viewer-content">
         <div v-if="isLoading" class="loading">Loading...</div>
         
         <div v-else-if="fileType === 'image'" class="media-container">
            <img :src="contentUrl" />
         </div>

         <div v-else-if="fileType === 'video'" class="media-container">
            <video :src="contentUrl" controls autoplay></video>
         </div>

         <div v-else-if="fileType === 'audio'" class="media-container audio-player">
            <div class="audio-icon"><Music :size="64" /></div>
            <audio :src="contentUrl" controls autoplay></audio>
         </div>

         <div v-else-if="fileType === 'document' && file.name.endsWith('.pdf')" class="media-container">
            <iframe :src="contentUrl" width="100%" height="100%"></iframe>
         </div>

         <div v-else-if="fileType === 'text' || fileType === 'document'" class="text-container">
            <pre>{{ textContent }}</pre>
         </div>

         <div v-else class="unavailable-state">
             <File :size="64" class="text-muted" />
             <p>Preview not available for this file type.</p>
             <button class="btn-primary mt-4" @click="handleDownload">Download File</button>
         </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.viewer-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.85);
    z-index: 100;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 2rem;
    backdrop-filter: blur(5px);
}

.viewer-container {
    background: var(--bg-surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    width: 100%;
    max-width: 1000px;
    height: 80vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.viewer-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 1rem;
    border-bottom: 1px solid var(--border);
    background: var(--bg-surface);
}

.viewer-header h3 {
    margin: 0;
    font-size: 1.1rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

.actions {
    display: flex;
    gap: 0.5rem;
}

.btn-icon {
    background: transparent;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.5rem;
    border-radius: var(--radius);
    transition: all 0.2s;
}

.btn-icon:hover {
    background: var(--bg-surface-hover);
    color: var(--text-main);
}

.viewer-content {
    flex: 1;
    overflow: auto;
    background: #000; /* Dark background for media */
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
}

.media-container {
    width: 100%;
    height: 100%;
    display: flex;
    align-items: center;
    justify-content: center;
}

img, video {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
}

iframe {
    border: none;
    background: #fff;
}

.text-container {
    width: 100%;
    height: 100%;
    padding: 1rem;
    background: var(--bg-app);
    color: var(--text-main);
    overflow: auto;
}

pre {
    font-family: monospace;
    white-space: pre-wrap;
    word-wrap: break-word;
}

.unavailable-state, .loading {
    color: var(--text-muted);
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
}

.audio-player {
    flex-direction: column;
    gap: 1rem;
    padding: 2rem;
}
</style>
