import { ref } from 'vue'

interface ClipboardState {
    ids: string[]
    sourceParentId: string
    sourceVaultName: string
    mode: 'cut' | 'copy'
}

const clipboard = ref<ClipboardState | null>(null)

export function useClipboard() {
    const setClipboard = (state: ClipboardState) => {
        clipboard.value = state
    }

    const clearClipboard = () => {
        clipboard.value = null
    }

    return {
        clipboard,
        setClipboard,
        clearClipboard
    }
}
