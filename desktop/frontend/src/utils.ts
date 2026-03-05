export const formatError = (e: unknown): string => {
    if (e instanceof Error) {
        // Sometimes the error message itself is JSON
        return formatError(e.message)
    }

    if (typeof e === 'string') {
        const trimmed = e.trim()
        if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
            try {
                const parsed = JSON.parse(trimmed)
                // Handle common error structures
                if (parsed.message) return formatError(parsed.message)
                if (parsed.error) return formatError(parsed.error)
                // If it's just a raw object, try to stringify it prettily or return specific fields
                return parsed.detail || JSON.stringify(parsed)
            } catch {
                // Not valid JSON, fall through
            }
        }
        return e
    }

    return String(e)
}

export const copyToClipboard = async (text: string): Promise<boolean> => {
    try {
        await navigator.clipboard.writeText(text);
        return true;
    } catch (err) {
        console.error('Failed to copy text: ', err);
        return false;
    }
}
