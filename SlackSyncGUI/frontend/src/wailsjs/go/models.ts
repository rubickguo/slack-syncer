export namespace main {
	
	export class Channel {
	    id: string;
	    name: string;
	    topic: string;
	    member_count: number;
	
	    static createFrom(source: any = {}) {
	        return new Channel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.topic = source["topic"];
	        this.member_count = source["member_count"];
	    }
	}

}

