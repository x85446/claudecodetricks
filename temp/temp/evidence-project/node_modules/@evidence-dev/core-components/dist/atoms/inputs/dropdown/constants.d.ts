/**
 * @typedef {Object} EvidenceDropdownContext
 * @property {({ value: any, label: string }) => () => void} registerOption
 */
export const DropdownContext: unique symbol;
export type EvidenceDropdownContext = {
    registerOption: ({ value: any, label: string }: {
        value: any;
        label: any;
    }) => () => void;
};
