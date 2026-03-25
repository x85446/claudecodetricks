export namespace postgresOptions {
    namespace host {
        export let title: string;
        export let type: string;
        export let secret: boolean;
        export let description: string;
        let _default: string;
        export { _default as default };
        export let required: boolean;
    }
    namespace port {
        let title_1: string;
        export { title_1 as title };
        let type_1: string;
        export { type_1 as type };
        let secret_1: boolean;
        export { secret_1 as secret };
        let description_1: string;
        export { description_1 as description };
        let _default_1: number;
        export { _default_1 as default };
        let required_1: boolean;
        export { required_1 as required };
    }
    namespace database {
        let title_2: string;
        export { title_2 as title };
        let type_2: string;
        export { type_2 as type };
        let secret_2: boolean;
        export { secret_2 as secret };
        let description_2: string;
        export { description_2 as description };
        let _default_2: string;
        export { _default_2 as default };
        let required_2: boolean;
        export { required_2 as required };
    }
    namespace user {
        let title_3: string;
        export { title_3 as title };
        let type_3: string;
        export { type_3 as type };
        let secret_3: boolean;
        export { secret_3 as secret };
        let description_3: string;
        export { description_3 as description };
        let required_3: boolean;
        export { required_3 as required };
    }
    namespace password {
        let title_4: string;
        export { title_4 as title };
        let type_4: string;
        export { type_4 as type };
        let secret_4: boolean;
        export { secret_4 as secret };
        let description_4: string;
        export { description_4 as description };
        let required_4: boolean;
        export { required_4 as required };
    }
    namespace ssl {
        let title_5: string;
        export { title_5 as title };
        let type_5: string;
        export { type_5 as type };
        let secret_5: boolean;
        export { secret_5 as secret };
        let description_5: string;
        export { description_5 as description };
        export let nest: boolean;
        export let children: {};
    }
    namespace schema {
        let title_6: string;
        export { title_6 as title };
        let type_6: string;
        export { type_6 as type };
        let secret_6: boolean;
        export { secret_6 as secret };
        let description_6: string;
        export { description_6 as description };
    }
}
