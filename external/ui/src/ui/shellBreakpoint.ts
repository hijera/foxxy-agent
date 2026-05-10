/** Viewports at most this width use stacked top nav and document scroll (see styles.css). */
export const SHELL_STACK_MAX_WIDTH_PX = 1199;

/** Pass to matchMedia for chat scroll shell behavior (align with CSS media queries). */
export const shellStackMaxWidthMediaQuery = `(max-width: ${SHELL_STACK_MAX_WIDTH_PX}px)`;
