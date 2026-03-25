export namespace prism_sql {
    export namespace comment {
        let pattern: RegExp;
        let lookbehind: boolean;
    }
    export let variable: (RegExp | {
        pattern: RegExp;
        greedy: boolean;
    })[];
    export namespace string {
        let pattern_1: RegExp;
        export { pattern_1 as pattern };
        export let greedy: boolean;
        let lookbehind_1: boolean;
        export { lookbehind_1 as lookbehind };
    }
    export namespace identifier {
        let pattern_2: RegExp;
        export { pattern_2 as pattern };
        let greedy_1: boolean;
        export { greedy_1 as greedy };
        let lookbehind_2: boolean;
        export { lookbehind_2 as lookbehind };
        export namespace inside {
            let punctuation: RegExp;
        }
    }
    let _function: RegExp;
    export { _function as function };
    export let keyword: RegExp;
    export let boolean: RegExp;
    export let number: RegExp;
    export let operator: RegExp;
    let punctuation_1: RegExp;
    export { punctuation_1 as punctuation };
}
