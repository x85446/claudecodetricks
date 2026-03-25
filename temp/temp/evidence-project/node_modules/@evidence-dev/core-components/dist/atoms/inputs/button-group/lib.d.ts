export function getButtonGroupContext(): {
    update: (v: ButtonGroupItem) => void;
    value: import("svelte/store").Readable<ButtonGroupItem>;
};
export function setButtonGroupContext(update: (v: ButtonGroupItem) => void, value: import("svelte/store").Readable<ButtonGroupItem>): void;
/**
 * @type {Record<string, ButtonGroupItem[]>}
 */
export const presets: Record<string, ButtonGroupItem[]>;
export type ButtonGroupItem = {
    valueLabel: string;
    value: string | boolean | number | Date;
};
