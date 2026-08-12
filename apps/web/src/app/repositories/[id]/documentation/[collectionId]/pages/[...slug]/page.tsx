import type {Metadata} from "next";
import {DocumentationReader} from "@/components/documentation-reader";
export const metadata:Metadata={title:"Published documentation"};
export default async function Page({params,searchParams}:{params:Promise<{id:string;collectionId:string;slug:string[]}>;searchParams:Promise<{version?:string|string[]}>}){const [{id,collectionId,slug},query]=await Promise.all([params,searchParams]);const version=Array.isArray(query.version)?query.version[0]:query.version;return <DocumentationReader repositoryId={id} collectionId={collectionId} slug={slug.join("/")} version={version}/>}
