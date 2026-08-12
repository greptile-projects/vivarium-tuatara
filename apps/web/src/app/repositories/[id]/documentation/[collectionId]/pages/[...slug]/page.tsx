import type {Metadata} from "next";
import {DocumentationReader} from "@/components/documentation-reader";
export const metadata:Metadata={title:"Published documentation"};
export default async function Page({params}:{params:Promise<{id:string;collectionId:string;slug:string[]}>}){const {id,collectionId,slug}=await params;return <DocumentationReader repositoryId={id} collectionId={collectionId} slug={slug.join("/")}/>}
