import { nextTick, watch } from 'vue';

const STICK_TOLERANCE = 128;

/**
 * Sticky TOP autoscroll for a message stream whose newest message sits at
 * the TOP, used by the non-standard modes.
 *
 * （2026-09-29 原来是「最新在底部」的 sticky bottom 版，Jonny：
 * 「便签模式的内容顺序调回最新的在顶部」→ 整套语义翻到顶上。全仓只有便签在用，
 * 所以直接改语义而不是加方向参数 —— 等第二个消费者出现再说。）
 *
 * Behavior:
 *  - Page load / refresh: nothing to do —— 最新的一条就在顶部，浏览器本来就停在那里。
 *  - Room switch: jump back to the top (instant, no animation).
 *  - New message / file arrives (item count grows): it is **prepended** above the
 *    viewport. Only follow (jump to top) while the reader is near the top ——
 *    那时他就是冲着「看新来的」来的；如果他往下翻着旧内容，别把他拽回去
 *   （往上插内容时浏览器自己的 scroll anchoring 会兜住视口，不闪）。
 *
 * The returned `pinToTop()` is intended for the composer: right after the user
 * sends, force-pin to the top so their own newest message is in view.
 *
 * @param {import('vue').Ref<HTMLElement|null>} elRef - scrollable stream container
 * @param {Object} opts
 * @param {() => Array} opts.items - getter for the received items array
 * @param {() => string} opts.room  - getter for the current room key
 * @param {number} [opts.tolerance=128] - distance from top treated as "near top"
 */
export function useStickyAutoscroll(elRef, { items, room, tolerance = STICK_TOLERANCE } = {}) {
    function node() {
        return elRef.value;
    }

    function nearTop() {
        const el = node();
        return el ? el.scrollTop < tolerance : false;
    }

    function pinToTop(smooth = true) {
        nextTick(() => {
            const el = node();
            if (!el) {
                return;
            }
            try {
                el.scrollTo({ top: 0, behavior: smooth ? 'smooth' : 'auto' });
            } catch (err) {
                el.scrollTop = 0;
            }
        });
    }

    if (room) {
        // Entering another room shows that room's newest message.
        watch(room, () => pinToTop(false));
    }

    if (items) {
        watch(
            () => items().length,
            (count, prevCount) => {
                if (count > prevCount && (prevCount === 0 || nearTop())) {
                    // A new message arrived. Follow only while the reader is near
                    // the top; if they're browsing history, leave them alone.
                    pinToTop(true);
                }
            },
        );
    }

    return { pinToTop, nearTop };
}
