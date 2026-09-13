import { nextTick, onMounted, watch } from 'vue';

const STICK_TOLERANCE = 128;

/**
 * Sticky bottom autoscroll for a message stream whose newest message sits at
 * the BOTTOM (right above the sticky composer), used by the non-standard modes.
 *
 * Behavior:
 *  - Page load / refresh: jump to the bottom (newest visible, no animation). This
 *    is what guarantees "after I refresh I always see the newest message".
 *  - Room switch: jump to the bottom (instant, no animation).
 *  - New message / file arrives (item count grows): follow along only while the
 *    reader is near the bottom; if they scrolled up into history, don't yank
 *    them back down.
 *  - The stream first filling up from empty (prev == 0): always jump to bottom.
 *
 * The returned `pinToBottom()` is intended for the composer: right after the
 * user sends, force-pin to the bottom so their own newest message is in view.
 *
 * @param {import('vue').Ref<HTMLElement|null>} elRef - scrollable stream container
 * @param {Object} opts
 * @param {() => Array} opts.items - getter for the received items array
 * @param {() => string} opts.room  - getter for the current room key
 * @param {number} [opts.tolerance=128] - distance from bottom treated as "near bottom"
 */
export function useStickyAutoscroll(elRef, { items, room, tolerance = STICK_TOLERANCE } = {}) {
    function node() {
        return elRef.value;
    }

    function nearBottom() {
        const el = node();
        return el ? el.scrollHeight - el.scrollTop - el.clientHeight < tolerance : false;
    }

    function pinToBottom(smooth = true) {
        nextTick(() => {
            const el = node();
            if (!el) {
                return;
            }
            try {
                el.scrollTo({ top: el.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
            } catch (err) {
                el.scrollTop = el.scrollHeight;
            }
        });
    }

    onMounted(() => {
        // Mount / refresh: always show the newest message first.
        pinToBottom(false);
    });

    if (room) {
        // Entering another room shows that room's newest message.
        watch(room, () => pinToBottom(false));
    }

    if (items) {
        watch(
            () => items().length,
            (count, prevCount) => {
                if (count > prevCount && (prevCount === 0 || nearBottom())) {
                    // A new message arrived. Follow only while the reader is near
                    // the bottom; if they're browsing history, leave them alone.
                    pinToBottom(true);
                }
            },
        );
    }

    return { pinToBottom, nearBottom };
}
